package models

import "time"

type AuthSecurityProfile struct {
	PreferredMFAMethod     string     `json:"preferredMfaMethod"`
	TOTPEnabled            bool       `json:"totpEnabled"`
	AvailableMethods       []string   `json:"availableMethods"`
	FederatedProvider      *string    `json:"federatedProvider,omitempty"`
	FederatedLinkedAt      *time.Time `json:"federatedLinkedAt,omitempty"`
	RecoveryCodesRemaining int        `json:"recoveryCodesRemaining"`
	RecoveryCodes          []string   `json:"recoveryCodes,omitempty"`
}

type TOTPSetupResponse struct {
	Issuer         string `json:"issuer"`
	AccountName    string `json:"accountName"`
	ManualEntryKey string `json:"manualEntryKey"`
	OTPAuthURL     string `json:"otpauthUrl"`
}

type RecoveryCodesResponse struct {
	Codes []string `json:"codes"`
}

type EmergencyResetRequestPayload struct {
	Email string `json:"email" binding:"required,email"`
}

type EmergencyResetConfirmPayload struct {
	Email      string `json:"email" binding:"required,email"`
	Code       string `json:"code" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"rememberMe"`
}
