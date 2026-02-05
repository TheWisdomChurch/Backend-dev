// internal/service/auth_service.go
package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

// AuthService implementation
type authServiceImpl struct {
	userRepo      repository.UserRepository
	otp           OTPService
	jwtSecret     string
	jwtExpiration time.Duration
	sender        EmailSender
	branding      email.Branding
	security      SecurityService
	trustedDevs   repository.TrustedDeviceRepository
	disableOTP    bool
}

var ErrAdminPending = errors.New("admin approval pending")
var ErrUserNotFound = errors.New("account not found")
var ErrWrongPassword = errors.New("incorrect password")

const (
	failedLoginThreshold  = 3
	failedLoginWindow     = 15 * time.Minute
	loginOTPPurposePrefix = "login:"
	otpEntryPath          = "/verify-otp"
	resetEntryPath        = "/reset-password"
	trustedDeviceTTL      = 30 * 24 * time.Hour
)

var allowedRoles = map[string]string{
	"admin":       "admin",
	"super_admin": "super_admin",
	"superadmin":  "super_admin",
	"super-admin": "super_admin",
	"super admin": "super_admin",
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo repository.UserRepository, otp OTPService, jwtSecret string, jwtExpiration time.Duration, sender EmailSender, branding email.Branding, security SecurityService, trustedDevs repository.TrustedDeviceRepository, disableOTP bool) AuthService {
	return &authServiceImpl{
		userRepo:      userRepo,
		otp:           otp,
		jwtSecret:     jwtSecret,
		jwtExpiration: jwtExpiration,
		sender:        sender,
		branding:      branding,
		security:      security,
		trustedDevs:   trustedDevs,
		disableOTP:    disableOTP,
	}
}

func (s *authServiceImpl) Login(email, password string, meta LoginMetadata) (*LoginResult, error) {
	emailAddr := normalizeEmail(email)
	if emailAddr == "" {
		return nil, errors.New("invalid credentials")
	}

	user, err := s.userRepo.FindByEmail(emailAddr)
	if err != nil {
		return nil, ErrUserNotFound
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	if isAdminRole(user.Role) && !user.AdminApproved {
		return nil, ErrAdminPending
	}

	if user.LastFailedLoginAt != nil && time.Since(*user.LastFailedLoginAt) > failedLoginWindow {
		user.FailedLoginCount = 0
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		s.recordFailedLogin(user, meta)
		return nil, ErrWrongPassword
	}

	user.FailedLoginCount = 0
	user.LastFailedLoginAt = nil

	needOTP := true
	if s.disableOTP {
		needOTP = false
	}

	// Step-up: untrusted/new/expired device requires OTP
	if !s.disableOTP && s.trustedDevs != nil && meta.DeviceID != "" {
		if dev, err := s.trustedDevs.Find(user.ID, meta.DeviceID); err == nil && dev != nil && dev.Trusted && dev.ExpiresAt.After(time.Now().UTC()) {
			needOTP = false
		}
	}

	if s.otp != nil && needOTP {
		challenge, err := generateLoginChallenge()
		if err != nil {
			return nil, errors.New("failed to start verification")
		}
		purpose := loginOTPPurposePrefix + challenge
		actionURL := s.buildOTPLink(otpEntryPath, purpose, user.Email)

		resp, err := s.otp.SendOTP(&models.SendOTPRequest{
			Email:       user.Email,
			Purpose:     purpose,
			ActionURL:   actionURL,
			ActionLabel: "Approve sign-in",
		})
		if err != nil {
			return nil, err
		}

		_ = s.userRepo.Update(user)

		expires := resp.ExpiresAt
		if s.security != nil {
			s.security.RecordEvent("otp_challenge", user, meta, map[string]interface{}{"purpose": purpose})
		}

		return &LoginResult{
			User:         sanitizeUser(user),
			OTPRequired:  true,
			OTPPurpose:   purpose,
			OTPExpiresAt: &expires,
			OTPActionURL: actionURL,
		}, nil
	}

	now := time.Now().UTC()
	user.LastLoginAt = &now
	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to update profile")
	}

	res := &LoginResult{
		User: sanitizeUser(user),
	}

	// Persist/refresh trusted device
	s.upsertTrustedDevice(user, meta, true)

	return res, nil
}

func (s *authServiceImpl) VerifyLoginOTP(email, code, purpose string, meta LoginMetadata) (*models.User, error) {
	if s.disableOTP {
		return nil, errors.New("otp is disabled")
	}
	if s.otp == nil {
		return nil, errors.New("otp service not configured")
	}

	emailAddr := normalizeEmail(email)
	if emailAddr == "" || strings.TrimSpace(purpose) == "" {
		return nil, errors.New("invalid request")
	}

	if !strings.HasPrefix(strings.TrimSpace(purpose), loginOTPPurposePrefix) {
		return nil, errors.New("invalid code")
	}

	resp, err := s.otp.VerifyOTP(&models.VerifyOTPRequest{
		Email:   emailAddr,
		Code:    code,
		Purpose: purpose,
	})
	if err != nil {
		// fallback: if purpose mismatched or missing, try latest login OTP
		if strings.Contains(err.Error(), "otp not found") && s.trustedDevs != nil {
			if latest, err2 := s.otp.(*otpService).repo.GetLatestActiveByPrefix(emailAddr, loginOTPPurposePrefix); err2 == nil {
				purpose = latest.Purpose
				resp, err = s.otp.VerifyOTP(&models.VerifyOTPRequest{
					Email:   emailAddr,
					Code:    code,
					Purpose: purpose,
				})
			}
		}
		if err != nil {
			return nil, err
		}
	}

	if resp == nil || !resp.Verified {
		return nil, errors.New("invalid code")
	}

	user, err := s.userRepo.FindByEmail(emailAddr)
	if err != nil {
		return nil, errors.New("user not found")
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.FailedLoginCount = 0
	user.LastFailedLoginAt = nil

	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to update profile")
	}

	s.upsertTrustedDevice(user, meta, true)

	return sanitizeUser(user), nil
}

func (s *authServiceImpl) recordFailedLogin(user *models.User, meta LoginMetadata) {
	if user == nil {
		return
	}

	now := time.Now().UTC()
	if user.LastFailedLoginAt == nil || now.Sub(*user.LastFailedLoginAt) > failedLoginWindow {
		user.FailedLoginCount = 1
	} else {
		user.FailedLoginCount++
	}
	user.LastFailedLoginAt = &now
	_ = s.userRepo.Update(user)

	if s.security != nil {
		s.security.RecordEvent("failed_login", user, meta, map[string]interface{}{
			"failed_count": user.FailedLoginCount,
		})
	}

	if user.FailedLoginCount == failedLoginThreshold {
		s.sendFailedLoginAlert(user, meta)
		if s.security != nil {
			s.security.NotifySuspiciousLogin(user, meta, "Multiple failed login attempts")
		}
	}
}

func (s *authServiceImpl) sendFailedLoginAlert(user *models.User, meta LoginMetadata) {
	if s.sender == nil || user == nil {
		return
	}

	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}

	body := email.RenderLoginAlertEmail(email.LoginAlertTemplateData{
		Branding:  s.branding,
		Email:     user.Email,
		IP:        meta.IP,
		UserAgent: meta.UserAgent,
		Timestamp: time.Now().UTC().Format(time.RFC1123),
	})

	subject := fmt.Sprintf("[%s] Multiple failed login attempts", appName)
	_ = s.sender.SendHTML(user.Email, subject, body)
}

func (s *authServiceImpl) RequestPasswordReset(email, actionURL string) (*models.SendOTPResponse, error) {
	if s.disableOTP {
		return nil, errors.New("otp is disabled")
	}
	if s.otp == nil {
		return nil, errors.New("otp service not configured")
	}

	emailAddr := normalizeEmail(email)
	if emailAddr == "" {
		return nil, errors.New("email is required")
	}

	user, err := s.userRepo.FindByEmail(emailAddr)
	if err != nil {
		return nil, errors.New("user not found")
	}

	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	challenge, err := generateLoginChallenge()
	if err != nil {
		return nil, errors.New("failed to generate reset token")
	}
	purpose := "password_reset:" + challenge

	link := strings.TrimSpace(actionURL)
	if link == "" {
		link = s.buildOTPLink(resetEntryPath, purpose, emailAddr)
	}

	resp, err := s.otp.SendOTP(&models.SendOTPRequest{
		Email:       emailAddr,
		Purpose:     purpose,
		ActionURL:   link,
		ActionLabel: "Reset password",
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *authServiceImpl) ResetPasswordWithOTP(emailStr, code, purpose, newPassword string) error {
	if s.disableOTP {
		return errors.New("otp is disabled")
	}
	if s.otp == nil {
		return errors.New("otp service not configured")
	}

	emailAddr := normalizeEmail(emailStr)
	if emailAddr == "" || strings.TrimSpace(newPassword) == "" {
		return errors.New("invalid request")
	}

	if !strings.HasPrefix(strings.TrimSpace(purpose), "password_reset:") {
		return errors.New("invalid code")
	}

	_, err := s.otp.VerifyOTP(&models.VerifyOTPRequest{
		Email:   emailAddr,
		Code:    code,
		Purpose: purpose,
	})
	if err != nil {
		return err
	}

	user, err := s.userRepo.FindByEmail(emailAddr)
	if err != nil {
		return errors.New("user not found")
	}
	if user == nil {
		return errors.New("user not found")
	}

	if !user.IsActive {
		return errors.New("account is deactivated")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("failed to hash password")
	}

	user.Password = string(hashedPassword)
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to update password")
	}

	// Send confirmation email
	if s.sender != nil {
		body := email.RenderPasswordChangedEmail(email.PasswordChangedTemplateData{
			Branding:  s.branding,
			Email:     user.Email,
			Timestamp: time.Now().UTC().Format(time.RFC1123),
			LoginURL:  s.buildOTPLink("/login", "", user.Email),
		})
		_ = s.sender.SendHTML(user.Email, "Your password was changed", body)
	}

	return nil
}

func (s *authServiceImpl) buildOTPLink(path, purpose, email string) string {
	base := strings.TrimSpace(s.branding.FrontendURL)
	if base == "" {
		base = strings.TrimSpace(s.branding.PublicURL)
	}
	if base == "" {
		return ""
	}

	parsed, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return ""
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	q := parsed.Query()
	if purpose != "" {
		q.Set("purpose", purpose)
	}
	if email != "" {
		q.Set("email", email)
	}
	parsed.RawQuery = q.Encode()

	return parsed.String()
}

func (s *authServiceImpl) upsertTrustedDevice(user *models.User, meta LoginMetadata, trusted bool) {
	if s.trustedDevs == nil || user == nil || meta.DeviceID == "" {
		return
	}
	dev := &models.TrustedDevice{
		UserID:     user.ID,
		DeviceID:   meta.DeviceID,
		LastIP:     meta.IP,
		UserAgent:  meta.UserAgent,
		Trusted:    trusted,
		LastSeenAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(trustedDeviceTTL),
	}
	_ = s.trustedDevs.Upsert(dev)
	if s.security != nil {
		s.security.RecordEvent("trusted_device_updated", user, meta, map[string]interface{}{"trusted": trusted})
	}
}

func (s *authServiceImpl) UntrustDevice(userID, deviceID string) error {
	if s.trustedDevs == nil || userID == "" || deviceID == "" {
		return nil
	}
	err := s.trustedDevs.MarkUntrusted(userID, deviceID)
	if s.security != nil {
		s.security.RecordEvent("trusted_device_untrusted", &models.User{ID: userID}, LoginMetadata{DeviceID: deviceID}, nil)
	}
	return err
}

// ResendLoginOTP issues a fresh login OTP for an already authenticated email/password attempt.
// It skips password validation; call this only after a recent OTP-required login response.
func (s *authServiceImpl) ResendLoginOTP(email string, meta LoginMetadata) (*LoginResult, error) {
	if s.disableOTP {
		return nil, errors.New("otp is disabled")
	}
	if s.otp == nil {
		return nil, errors.New("otp service not configured")
	}
	emailAddr := normalizeEmail(email)
	if emailAddr == "" {
		return nil, ErrUserNotFound
	}
	user, err := s.userRepo.FindByEmail(emailAddr)
	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if isAdminRole(user.Role) && !user.AdminApproved {
		return nil, ErrAdminPending
	}

	challenge, err := generateLoginChallenge()
	if err != nil {
		return nil, errors.New("failed to start verification")
	}
	purpose := loginOTPPurposePrefix + challenge
	actionURL := s.buildOTPLink(otpEntryPath, purpose, user.Email)

	resp, err := s.otp.SendOTP(&models.SendOTPRequest{
		Email:       user.Email,
		Purpose:     purpose,
		ActionURL:   actionURL,
		ActionLabel: "Approve sign-in",
	})
	if err != nil {
		return nil, err
	}

	expires := resp.ExpiresAt
	if s.security != nil {
		s.security.RecordEvent("otp_resend", user, meta, map[string]interface{}{"purpose": purpose})
	}

	return &LoginResult{
		User:         sanitizeUser(user),
		OTPRequired:  true,
		OTPPurpose:   purpose,
		OTPExpiresAt: &expires,
		OTPActionURL: actionURL,
	}, nil
}

func generateLoginChallenge() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func sanitizeUser(user *models.User) *models.User {
	if user == nil {
		return nil
	}
	user.Password = ""
	return user
}

func (s *authServiceImpl) Register(firstName, lastName, email, password, role string) (interface{}, error) {
	emailNorm := normalizeEmail(email)
	if emailNorm == "" {
		return nil, errors.New("invalid email")
	}
	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}

	// Check if user already exists
	existingUser, _ := s.userRepo.FindByEmail(emailNorm)
	if existingUser != nil {
		return nil, errors.New("user already exists")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Create user object
	user := &models.User{
		FirstName: firstName,
		LastName:  lastName,
		Email:     emailNorm,
		Password:  string(hashedPassword),
		Role:      role,
		IsActive:  true,
		AdminApproved: func() bool {
			if role == "admin" {
				return false
			}
			return true
		}(),
	}

	// Save user
	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	s.sendAdminWelcome(user)

	// Remove password from response
	user.Password = ""
	return user, nil
}

func normalizeRole(role string) (string, error) {
	cleaned := strings.ToLower(strings.TrimSpace(role))
	if normalized, ok := allowedRoles[cleaned]; ok {
		return normalized, nil
	}
	return "", fmt.Errorf("invalid role: %s", role)
}

func (s *authServiceImpl) sendAdminWelcome(user *models.User) {
	if s.sender == nil || user == nil || !isAdminRole(user.Role) {
		return
	}
	fullName := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	body := email.RenderAdminWelcomeEmail(email.AdminWelcomeTemplateData{
		Branding:      s.branding,
		RecipientName: fullName,
		Role:          user.Role,
	})
	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}
	subject := fmt.Sprintf("Welcome to %s Admin", appName)
	_ = s.sender.SendHTML(user.Email, subject, body)
}

func isAdminRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "super_admin", "superadmin", "super-admin":
		return true
	default:
		return false
	}
}

func (s *authServiceImpl) GetUserByID(userID string) (interface{}, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, err
	}

	// Check if user is active
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	// Remove password from response
	user.Password = ""
	return user, nil
}

// UpdateProfile updates user profile information
func (s *authServiceImpl) UpdateProfile(userID, firstName, lastName, email, username string) (interface{}, error) {
	// Get current user
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	// Update fields if provided
	updated := false
	if firstName != "" && firstName != user.FirstName {
		user.FirstName = firstName
		updated = true
	}

	if lastName != "" && lastName != user.LastName {
		user.LastName = lastName
		updated = true
	}

	if email != "" && email != user.Email {
		// Check if email already exists for another user
		existingUser, _ := s.userRepo.FindByEmail(email)
		if existingUser != nil && existingUser.ID != userID {
			return nil, errors.New("email already in use")
		}
		user.Email = email
		updated = true
	}

	// Only update if something changed
	if updated {
		user.UpdatedAt = time.Now()

		// Save updated user
		if err := s.userRepo.Update(user); err != nil {
			return nil, errors.New("failed to update profile")
		}
	}

	// Remove password from response
	user.Password = ""
	return user, nil
}

// ChangePassword changes user password
func (s *authServiceImpl) ChangePassword(userID, currentPassword, newPassword string) error {
	// Get user
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive {
		return errors.New("account is deactivated")
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("failed to hash password")
	}

	// Update password
	user.Password = string(hashedPassword)
	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to update password")
	}

	return nil
}

// DeleteAccount deletes user account (hard delete)
func (s *authServiceImpl) DeleteAccount(userID string) error {
	// Get user
	_, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}

	if err := s.userRepo.DeleteHard(userID); err != nil {
		return errors.New("failed to delete account")
	}

	return nil
}

// ClearData clears user data (placeholder implementation)
func (s *authServiceImpl) ClearData(userID string) error {
	// Get user to verify existence
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive {
		return errors.New("account is deactivated")
	}

	// This is a placeholder - implement based on your application needs
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to clear data")
	}

	return nil
}

// Helper method to generate JWT token
func (s *authServiceImpl) generateToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"exp":     time.Now().Add(s.jwtExpiration).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}
