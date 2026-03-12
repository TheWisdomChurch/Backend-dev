package service

import (
	"errors"
	"strings"
	"time"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/models"
)

func normalizeMFAMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", "email", "email_otp", "otp":
		return "email_otp"
	case "totp", "authenticator", "authenticator_app":
		return "totp"
	default:
		return ""
	}
}

func (s *authServiceImpl) GetSecurityProfile(userID string) (*models.AuthSecurityProfile, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	return buildSecurityProfile(user), nil
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
		issuer = "Wisdom House"
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

	user.TOTPSecretEnc = &encrypted
	user.TOTPPendingEnc = nil
	user.TOTPEnabled = true
	user.PreferredMFAMethod = "totp"
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to enable authenticator")
	}

	return buildSecurityProfile(user), nil
}

func (s *authServiceImpl) DisableTOTP(userID, code string) (*models.AuthSecurityProfile, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
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
	user.PreferredMFAMethod = "email_otp"
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to disable authenticator")
	}

	return buildSecurityProfile(user), nil
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
	if normalized == "totp" && !user.TOTPEnabled {
		return nil, errors.New("enable authenticator app before selecting totp")
	}

	user.PreferredMFAMethod = normalized
	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.Update(user); err != nil {
		return nil, errors.New("failed to update mfa preference")
	}

	return buildSecurityProfile(user), nil
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

func buildSecurityProfile(user *models.User) *models.AuthSecurityProfile {
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

	return &models.AuthSecurityProfile{
		PreferredMFAMethod: preferred,
		TOTPEnabled:        user != nil && user.TOTPEnabled,
		AvailableMethods:   methods,
		FederatedProvider:  federatedProvider,
		FederatedLinkedAt:  federatedLinkedAt,
	}
}
