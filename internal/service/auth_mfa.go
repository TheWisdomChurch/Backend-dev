package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/models"
)

func normalizeMFAMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", "email", "email_otp", "otp":
		return "email_otp"
	case "totp", "authenticator", "authenticator_app":
		return "totp"
	case "recovery_code", "recovery", "backup_code", "backup":
		return "recovery_code"
	default:
		return ""
	}
}

type RecoveryCodeRecord struct {
	Code   string     `json:"code"`
	Used   bool       `json:"used"`
	UsedAt *time.Time `json:"used_at,omitempty"`
}

func (s *authServiceImpl) loadRecoveryCodes(user *models.User) []RecoveryCodeRecord {
	if s.totpProtector == nil || user == nil || user.TOTPRecoveryCodesEnc == nil || strings.TrimSpace(*user.TOTPRecoveryCodesEnc) == "" {
		return nil
	}
	decrypted, err := s.totpProtector.DecryptString(*user.TOTPRecoveryCodesEnc)
	if err != nil {
		return nil
	}
	var records []RecoveryCodeRecord
	if err := json.Unmarshal([]byte(decrypted), &records); err != nil {
		return nil
	}
	return records
}

func (s *authServiceImpl) storeRecoveryCodes(user *models.User, records []RecoveryCodeRecord) error {
	if s.totpProtector == nil || user == nil {
		return errors.New("authenticator protector not configured")
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	enc, err := s.totpProtector.EncryptString(string(raw))
	if err != nil {
		return err
	}
	user.TOTPRecoveryCodesEnc = &enc
	return nil
}

func (s *authServiceImpl) countRemainingRecoveryCodes(user *models.User) int {
	records := s.loadRecoveryCodes(user)
	count := 0
	for _, r := range records {
		if !r.Used {
			count++
		}
	}
	return count
}

func (s *authServiceImpl) verifyAndConsumeRecoveryCode(user *models.User, inputCode string) bool {
	if user == nil || !user.TOTPEnabled {
		return false
	}
	normInput := authutil.NormalizeRecoveryCode(inputCode)
	if len(normInput) != 8 {
		return false
	}
	records := s.loadRecoveryCodes(user)
	if len(records) == 0 {
		return false
	}

	foundIdx := -1
	for i, r := range records {
		if !r.Used && authutil.NormalizeRecoveryCode(r.Code) == normInput {
			foundIdx = i
			break
		}
	}
	if foundIdx == -1 {
		return false
	}

	records[foundIdx].Used = true
	now := time.Now().UTC()
	records[foundIdx].UsedAt = &now

	if err := s.storeRecoveryCodes(user, records); err != nil {
		return false
	}
	user.UpdatedAt = now
	if err := s.userRepo.Update(user); err != nil {
		return false
	}

	if s.security != nil {
		s.security.RecordEvent("mfa_recovery_code_used", user, LoginMetadata{}, map[string]interface{}{
			"remaining": s.countRemainingRecoveryCodes(user),
		})
	}
	return true
}

func (s *authServiceImpl) GetSecurityProfile(userID string) (*models.AuthSecurityProfile, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	return s.buildSecurityProfile(user), nil
}

func (s *authServiceImpl) BeginTOTPSetup(userID string) (*models.TOTPSetupResponse, error) {
	if s.disableOTP {
		return nil, errors.New("multi-factor authentication is disabled")
	}
	if s.totpProtector == nil {
		return nil, errors.New("authenticator setup is not configured")
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if user.TOTPEnabled {
		return nil, errors.New("authenticator app is already enabled")
	}

	var secret string
	if user.TOTPPendingEnc != nil && strings.TrimSpace(*user.TOTPPendingEnc) != "" {
		if decrypted, decryptErr := s.totpProtector.DecryptString(*user.TOTPPendingEnc); decryptErr == nil {
			secret = strings.TrimSpace(decrypted)
		}
	}

	if secret == "" {
		var err error
		secret, err = authutil.GenerateTOTPSecret(20)
		if err != nil {
			return nil, errors.New("failed to generate authenticator secret")
		}

		encrypted, err := s.totpProtector.EncryptString(secret)
		if err != nil {
			return nil, errors.New("failed to secure authenticator secret")
		}

		user.TOTPPendingEnc = &encrypted
		user.UpdatedAt = time.Now().UTC()
		if err := s.userRepo.Update(user); err != nil {
			return nil, errors.New("failed to prepare authenticator setup")
		}
	}

	issuer := strings.TrimSpace(s.mfaIssuer)
	if issuer == "" {
		issuer = strings.TrimSpace(s.branding.AppName)
	}
	if issuer == "" {
		issuer = "The Wisdom Church"
	}

	accountName := strings.TrimSpace(user.Email)
	return &models.TOTPSetupResponse{
		Issuer:         issuer,
		AccountName:    accountName,
		ManualEntryKey: secret,
		OTPAuthURL:     authutil.BuildTOTPAuthURL(issuer, accountName, secret),
	}, nil
}

// ReconfigureTOTP initiates device replacement / re-registration for an existing user.
func (s *authServiceImpl) ReconfigureTOTP(userID string) (*models.TOTPSetupResponse, error) {
	if s.disableOTP {
		return nil, errors.New("multi-factor authentication is disabled")
	}
	if s.totpProtector == nil {
		return nil, errors.New("authenticator setup is not configured")
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	secret, err := authutil.GenerateTOTPSecret(20)
	if err != nil {
		return nil, errors.New("failed to generate authenticator secret")
	}

	encrypted, err := s.totpProtector.EncryptString(secret)
	if err != nil {
		return nil, errors.New("failed to secure authenticator secret")
	}

	user.TOTPPendingEnc = &encrypted
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to prepare authenticator setup")
	}

	issuer := strings.TrimSpace(s.mfaIssuer)
	if issuer == "" {
		issuer = strings.TrimSpace(s.branding.AppName)
	}
	if issuer == "" {
		issuer = "The Wisdom Church"
	}

	accountName := strings.TrimSpace(user.Email)
	return &models.TOTPSetupResponse{
		Issuer:         issuer,
		AccountName:    accountName,
		ManualEntryKey: secret,
		OTPAuthURL:     authutil.BuildTOTPAuthURL(issuer, accountName, secret),
	}, nil
}

func (s *authServiceImpl) EnableTOTP(userID, code string) (*models.AuthSecurityProfile, error) {
	if s.disableOTP {
		return nil, errors.New("multi-factor authentication is disabled")
	}
	if s.totpProtector == nil {
		return nil, errors.New("authenticator setup is not configured")
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if user.TOTPPendingEnc == nil || strings.TrimSpace(*user.TOTPPendingEnc) == "" {
		return nil, errors.New("authenticator setup has not been started")
	}

	secret, err := s.totpProtector.DecryptString(*user.TOTPPendingEnc)
	if err != nil {
		return nil, errors.New("pending authenticator setup is invalid")
	}
	if !authutil.VerifyTOTP(secret, code, time.Now().UTC(), 1) {
		return nil, errors.New("invalid code")
	}

	encrypted, err := s.totpProtector.EncryptString(secret)
	if err != nil {
		return nil, errors.New("failed to enable authenticator")
	}

	// Generate 8 fresh backup recovery codes
	recoveryCodes, err := authutil.GenerateRecoveryCodes(8)
	if err != nil {
		return nil, errors.New("failed to generate recovery codes")
	}
	records := make([]RecoveryCodeRecord, len(recoveryCodes))
	for i, rc := range recoveryCodes {
		records[i] = RecoveryCodeRecord{Code: rc, Used: false}
	}
	if err := s.storeRecoveryCodes(user, records); err != nil {
		return nil, errors.New("failed to store recovery codes")
	}

	user.TOTPSecretEnc = &encrypted
	user.TOTPPendingEnc = nil
	user.TOTPEnabled = true
	user.PreferredMFAMethod = "totp"
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to enable authenticator")
	}

	profile := s.buildSecurityProfile(user)
	profile.RecoveryCodes = recoveryCodes
	return profile, nil
}

func (s *authServiceImpl) DisableTOTP(userID, code string) (*models.AuthSecurityProfile, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if isAdminRole(user.Role) {
		return nil, errors.New("admin accounts cannot disable authenticator app")
	}
	if !user.TOTPEnabled || user.TOTPSecretEnc == nil || strings.TrimSpace(*user.TOTPSecretEnc) == "" {
		return nil, errors.New("authenticator app is not enabled")
	}
	if !s.verifyStoredTOTP(user, code) {
		return nil, errors.New("invalid code")
	}

	user.TOTPEnabled = false
	user.TOTPSecretEnc = nil
	user.TOTPPendingEnc = nil
	user.TOTPRecoveryCodesEnc = nil
	user.PreferredMFAMethod = "email_otp"
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to disable authenticator")
	}

	return s.buildSecurityProfile(user), nil
}

func (s *authServiceImpl) SetPreferredMFAMethod(userID, method string) (*models.AuthSecurityProfile, error) {
	normalized := normalizeMFAMethod(method)
	if normalized == "" {
		return nil, errors.New("unsupported mfa method")
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if isAdminRole(user.Role) && normalized != "totp" {
		return nil, errors.New("admin accounts must use authenticator app (totp)")
	}
	if normalized == "totp" && !user.TOTPEnabled {
		return nil, errors.New("enable authenticator app before selecting totp")
	}

	user.PreferredMFAMethod = normalized
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to update mfa preference")
	}

	return s.buildSecurityProfile(user), nil
}

// GenerateRecoveryCodes regenerates a fresh set of 8 recovery codes for the user.
func (s *authServiceImpl) GenerateRecoveryCodes(userID string) ([]string, error) {
	if s.disableOTP {
		return nil, errors.New("multi-factor authentication is disabled")
	}
	if s.totpProtector == nil {
		return nil, errors.New("authenticator setup is not configured")
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if !user.TOTPEnabled {
		return nil, errors.New("authenticator app must be enabled before generating recovery codes")
	}

	codes, err := authutil.GenerateRecoveryCodes(8)
	if err != nil {
		return nil, errors.New("failed to generate recovery codes")
	}

	records := make([]RecoveryCodeRecord, len(codes))
	for i, c := range codes {
		records[i] = RecoveryCodeRecord{Code: c, Used: false}
	}

	if err := s.storeRecoveryCodes(user, records); err != nil {
		return nil, errors.New("failed to store recovery codes")
	}
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to save recovery codes")
	}

	if s.security != nil {
		s.security.RecordEvent("mfa_recovery_codes_regenerated", user, LoginMetadata{}, nil)
	}

	return codes, nil
}

// RequestEmergencyMFAReset initiates an email OTP verification to reset a lost 2FA device.
func (s *authServiceImpl) RequestEmergencyMFAReset(emailAddr string) error {
	trimmedEmail := strings.ToLower(strings.TrimSpace(emailAddr))
	if trimmedEmail == "" {
		return nil
	}

	user, err := s.userRepo.FindByEmail(trimmedEmail)
	if err != nil || user == nil || !user.IsActive || !user.TOTPEnabled {
		// Silent no-op to prevent email enumeration
		return nil
	}

	if s.otp == nil {
		return errors.New("otp service not configured")
	}

	_, err = s.otp.SendOTP(&models.SendOTPRequest{
		Email:   user.Email,
		Purpose: "mfa_device_reset",
	})
	return err
}

// ConfirmEmergencyMFAReset resets 2FA after verifying the email OTP and account password.
func (s *authServiceImpl) ConfirmEmergencyMFAReset(emailAddr, code, password string) (*models.User, error) {
	trimmedEmail := strings.ToLower(strings.TrimSpace(emailAddr))
	if trimmedEmail == "" || strings.TrimSpace(code) == "" || strings.TrimSpace(password) == "" {
		return nil, errors.New("email, verification code, and password are required")
	}

	user, err := s.userRepo.FindByEmail(trimmedEmail)
	if err != nil || user == nil {
		return nil, errors.New("invalid credentials")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	if !user.TOTPEnabled {
		return nil, errors.New("authenticator is not enabled for this account")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.New("incorrect password")
	}

	// Verify OTP
	if s.otp == nil {
		return nil, errors.New("otp service not configured")
	}
	resp, err := s.otp.VerifyOTP(&models.VerifyOTPRequest{
		Email:   user.Email,
		Code:    strings.TrimSpace(code),
		Purpose: "mfa_device_reset",
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.Verified {
		return nil, errors.New("invalid verification code")
	}

	// Reset TOTP, pending setups, and recovery codes
	user.TOTPEnabled = false
	user.TOTPSecretEnc = nil
	user.TOTPPendingEnc = nil
	user.TOTPRecoveryCodesEnc = nil
	user.PreferredMFAMethod = "email_otp"
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to reset authenticator device")
	}

	if s.security != nil {
		s.security.RecordEvent("mfa_emergency_device_reset_completed", user, LoginMetadata{}, nil)
	}

	return sanitizeUser(user), nil
}

// AdminResetUser2FA resets 2FA for a user (called by a Super Admin).
func (s *authServiceImpl) AdminResetUser2FA(targetUserID string) error {
	user, err := s.userRepo.FindByID(targetUserID)
	if err != nil || user == nil {
		return errors.New("user not found")
	}

	user.TOTPEnabled = false
	user.TOTPSecretEnc = nil
	user.TOTPPendingEnc = nil
	user.TOTPRecoveryCodesEnc = nil
	user.PreferredMFAMethod = "email_otp"
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to reset user 2FA")
	}

	if s.security != nil {
		s.security.RecordEvent("admin_reset_user_2fa", user, LoginMetadata{}, nil)
	}

	return nil
}

func (s *authServiceImpl) verifyStoredTOTP(user *models.User, code string) bool {
	if s.totpProtector == nil || user == nil || user.TOTPSecretEnc == nil || strings.TrimSpace(*user.TOTPSecretEnc) == "" {
		return false
	}

	secret, err := s.totpProtector.DecryptString(*user.TOTPSecretEnc)
	if err != nil {
		return false
	}

	return authutil.VerifyTOTP(secret, code, time.Now().UTC(), 1)
}

func (s *authServiceImpl) buildSecurityProfile(user *models.User) *models.AuthSecurityProfile {
	preferred := "email_otp"
	var federatedProvider *string
	var federatedLinkedAt *time.Time
	if user != nil {
		if normalized := normalizeMFAMethod(user.PreferredMFAMethod); normalized != "" {
			preferred = normalized
		}
		federatedProvider = user.FederatedProvider
		federatedLinkedAt = user.FederatedLinkedAt
	}

	methods := []string{"email_otp"}
	if user != nil && user.TOTPEnabled {
		methods = append(methods, "totp")
	}

	remaining := 0
	if user != nil {
		remaining = s.countRemainingRecoveryCodes(user)
	}

	return &models.AuthSecurityProfile{
		PreferredMFAMethod:     preferred,
		TOTPEnabled:            user != nil && user.TOTPEnabled,
		AvailableMethods:       methods,
		FederatedProvider:      federatedProvider,
		FederatedLinkedAt:      federatedLinkedAt,
		RecoveryCodesRemaining: remaining,
	}
}
