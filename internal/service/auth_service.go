// internal/service/auth_service.go
package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

// AuthService implementation
type authServiceImpl struct {
	db              *gorm.DB // for transaction management
	userRepo        repository.UserRepository
	otp             OTPService
	sender          EmailSender
	branding        email.Branding
	security        SecurityService
	trustedDevs     repository.TrustedDeviceRepository
	disableOTP      bool
	disableLoginOTP bool
	approvalSvc     ApprovalService
	notifySvc       AdminNotificationService
	totpProtector   *authutil.Protector
	mfaIssuer       string
}

var ErrAdminPending = errors.New("admin approval pending")
var ErrUserNotFound = errors.New("account not found")
var ErrWrongPassword = errors.New("incorrect password")
var ErrAdminTOTPRequired = errors.New("admin accounts with authenticator enabled must complete totp verification")

const (
	failedLoginThreshold  = 5
	failedLoginWindow     = 15 * time.Minute
	accountLockDuration   = 30 * time.Minute
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
func NewAuthService(db *gorm.DB, userRepo repository.UserRepository, otp OTPService, sender EmailSender, branding email.Branding, security SecurityService, trustedDevs repository.TrustedDeviceRepository, disableOTP bool, disableLoginOTP bool, approvalSvc ApprovalService, notifySvc AdminNotificationService, mfaIssuer string, authSecret string) AuthService {
	var protector *authutil.Protector
	if p, err := authutil.NewProtector(authSecret); err == nil {
		protector = p
	}
	return &authServiceImpl{
		db:              db,
		userRepo:        userRepo,
		otp:             otp,
		sender:          sender,
		branding:        branding,
		security:        security,
		trustedDevs:     trustedDevs,
		disableOTP:      disableOTP,
		disableLoginOTP: disableLoginOTP,
		approvalSvc:     approvalSvc,
		notifySvc:       notifySvc,
		totpProtector:   protector,
		mfaIssuer:       strings.TrimSpace(mfaIssuer),
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

	// Auto-unlock if the lock period has passed.
	if user.IsLocked && user.LockedUntil != nil && time.Now().UTC().After(*user.LockedUntil) {
		user.IsLocked = false
		user.LockedUntil = nil
		user.FailedLoginCount = 0
		_ = s.userRepo.Update(user)
	}

	if user.IsLocked {
		remaining := "a few minutes"
		if user.LockedUntil != nil {
			d := time.Until(*user.LockedUntil).Round(time.Minute)
			if d > 0 {
				remaining = d.String()
			}
		}
		return nil, fmt.Errorf("account temporarily locked due to too many failed login attempts. Try again in %s", remaining)
	}

	// Reset stale failed-login counter.
	if user.LastFailedLoginAt != nil && time.Since(*user.LastFailedLoginAt) > failedLoginWindow {
		user.FailedLoginCount = 0
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		s.recordFailedLogin(user, meta)
		return nil, ErrWrongPassword
	}

	user.FailedLoginCount = 0
	user.LastFailedLoginAt = nil

	return s.completePrimaryLogin(user, meta, "password")
}

func (s *authServiceImpl) VerifyLoginMFA(email, code, purpose, method string, meta LoginMetadata) (*models.User, string, error) {
	emailAddr := normalizeEmail(email)
	if emailAddr == "" {
		return nil, "", errors.New("invalid request")
	}

	user, err := s.userRepo.FindByEmail(emailAddr)
	if err != nil || user == nil {
		return nil, "", errors.New("user not found")
	}
	if !user.IsActive {
		return nil, "", errors.New("account is deactivated")
	}

	method = normalizeMFAMethod(method)
	if isAdminRole(user.Role) && user.TOTPEnabled {
		// Admin sessions must be bound to a TOTP verification.
		method = "totp"
	}
	if method == "" {
		if strings.HasPrefix(strings.TrimSpace(purpose), loginOTPPurposePrefix) {
			method = "email_otp"
		} else if user.TOTPEnabled && normalizeMFAMethod(user.PreferredMFAMethod) == "totp" {
			method = "totp"
		} else {
			method = "email_otp"
		}
	}

	switch method {
	case "totp":
		if !s.verifyStoredTOTP(user, code) {
			return nil, "", errors.New("invalid code")
		}
	case "email_otp":
		if s.disableOTP || s.disableLoginOTP {
			return nil, "", errors.New("otp is disabled")
		}
		if s.otp == nil {
			return nil, "", errors.New("otp service not configured")
		}
		purpose = strings.TrimSpace(purpose)
		if purpose == "" || !strings.HasPrefix(purpose, loginOTPPurposePrefix) {
			return nil, "", errors.New("invalid code")
		}

		resp, err := s.otp.VerifyOTP(&models.VerifyOTPRequest{
			Email:   emailAddr,
			Code:    code,
			Purpose: purpose,
		})
		if err != nil {
			return nil, "", err
		}
		if resp == nil || !resp.Verified {
			return nil, "", errors.New("invalid code")
		}
	default:
		return nil, "", errors.New("unsupported verification method")
	}

	if err := s.markLoginComplete(user, meta); err != nil {
		return nil, "", err
	}

	return sanitizeUser(user), method, nil
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

	// Lock the account when the threshold is reached.
	if user.FailedLoginCount >= failedLoginThreshold {
		user.IsLocked = true
		lockedUntil := now.Add(accountLockDuration)
		user.LockedUntil = &lockedUntil
	}

	if err := s.userRepo.Update(user); err != nil {
		slog.Warn("auth_service: failed to persist failed-login state", "user_id", user.ID, "error", err)
	}

	if s.security != nil {
		s.security.RecordEvent("failed_login", user, meta, map[string]interface{}{
			"failed_count": user.FailedLoginCount,
			"locked":       user.IsLocked,
		})
	}

	if user.FailedLoginCount >= failedLoginThreshold {
		s.sendFailedLoginAlert(user, meta)
		if s.security != nil {
			s.security.NotifySuspiciousLogin(user, meta, "Account locked after multiple failed login attempts")
		}
	}
}

func (s *authServiceImpl) completePrimaryLogin(user *models.User, meta LoginMetadata, authMethod string) (*LoginResult, error) {
	if user == nil {
		return nil, errors.New("user not found")
	}

	// Require a TOTP step for admin/super_admin users once authenticator is enabled.
	// This keeps issued session auth_method aligned with admin middleware enforcement.
	if isAdminRole(user.Role) && user.TOTPEnabled {
		if s.disableOTP {
			return nil, errors.New("multi-factor authentication is disabled")
		}
		if s.security != nil {
			s.security.RecordEvent("totp_challenge", user, meta, nil)
		}
		return &LoginResult{
			User:        sanitizeUser(user),
			OTPRequired: true,
			MFAMethod:   "totp",
		}, nil
	}

	if s.isTrustedDevice(user, meta) {
		if err := s.markLoginComplete(user, meta); err != nil {
			return nil, err
		}
		return &LoginResult{
			User:       sanitizeUser(user),
			AuthMethod: authMethod,
		}, nil
	}

	method := s.resolveMFAMethod(user)
	switch method {
	case "totp":
		if s.disableOTP {
			break
		}
		if !user.TOTPEnabled {
			break
		}
		if s.security != nil {
			s.security.RecordEvent("totp_challenge", user, meta, nil)
		}
		return &LoginResult{
			User:        sanitizeUser(user),
			OTPRequired: true,
			MFAMethod:   "totp",
		}, nil
	case "email_otp":
		if s.disableOTP || s.disableLoginOTP {
			break
		}
		if s.otp == nil {
			return nil, errors.New("otp service not configured")
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

		if s.security != nil {
			s.security.RecordEvent("otp_challenge", user, meta, map[string]interface{}{"purpose": purpose})
		}

		expires := resp.ExpiresAt
		return &LoginResult{
			User:         sanitizeUser(user),
			OTPRequired:  true,
			MFAMethod:    "email_otp",
			OTPPurpose:   purpose,
			OTPExpiresAt: &expires,
			OTPActionURL: actionURL,
		}, nil
	}

	if err := s.markLoginComplete(user, meta); err != nil {
		return nil, err
	}

	return &LoginResult{
		User:       sanitizeUser(user),
		AuthMethod: authMethod,
	}, nil
}

func (s *authServiceImpl) markLoginComplete(user *models.User, meta LoginMetadata) error {
	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.FailedLoginCount = 0
	user.LastFailedLoginAt = nil

	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to update profile")
	}

	s.upsertTrustedDevice(user, meta, true)
	return nil
}

func (s *authServiceImpl) isTrustedDevice(user *models.User, meta LoginMetadata) bool {
	if s.trustedDevs == nil || user == nil || strings.TrimSpace(meta.DeviceID) == "" {
		return false
	}
	dev, err := s.trustedDevs.Find(user.ID, meta.DeviceID)
	return err == nil && dev != nil && dev.Trusted && dev.ExpiresAt.After(time.Now().UTC())
}

func (s *authServiceImpl) resolveMFAMethod(user *models.User) string {
	if user == nil {
		return "email_otp"
	}
	method := normalizeMFAMethod(user.PreferredMFAMethod)
	switch method {
	case "totp":
		if user.TOTPEnabled {
			return "totp"
		}
	}
	return "email_otp"
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
		return nil, ErrUserNotFound
	}
	if user == nil {
		return nil, ErrUserNotFound
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

	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return errors.New("invalid code")
	}
	if !strings.HasPrefix(purpose, "password_reset") {
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
	base := strings.TrimSpace(s.branding.AdminPortalURL)
	if base == "" {
		base = strings.TrimSpace(s.branding.FrontendURL)
	}
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
	if s.disableOTP || s.disableLoginOTP {
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
	if s.resolveMFAMethod(user) == "totp" {
		return nil, errors.New("authenticator app verification is enabled for this account")
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
		MFAMethod:    "email_otp",
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

	// Wrap user creation + approval request in a transaction for atomicity.
	if s.db != nil {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			txRepo := s.userRepo.WithTx(tx)
			if err := txRepo.Create(user); err != nil {
				return err
			}
			if needsAdminApproval(user) {
				if err := requestAdminApprovalTx(s.approvalSvc, user); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	} else {
		// Fallback when db is not injected (tests / legacy path).
		if err := s.userRepo.Create(user); err != nil {
			return nil, err
		}
	}

	// Side-effects outside the transaction (email, non-critical notifications).
	if needsAdminApproval(user) {
		notifyAdminNewRegistration(s.notifySvc, user)
	} else {
		s.sendAdminWelcome(user)
	}

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
	if needsAdminApproval(user) {
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
