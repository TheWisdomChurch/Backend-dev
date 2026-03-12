package models

import "time"

type AuthSecurityProfile struct {
	PreferredMFAMethod string     `json:"preferredMfaMethod"`
	TOTPEnabled        bool       `json:"totpEnabled"`
	AvailableMethods   []string   `json:"availableMethods"`
	FederatedProvider  *string    `json:"federatedProvider,omitempty"`
	FederatedLinkedAt  *time.Time `json:"federatedLinkedAt,omitempty"`
}

type TOTPSetupResponse struct {
	Issuer         string `json:"issuer"`
	AccountName    string `json:"accountName"`
	ManualEntryKey string `json:"manualEntryKey"`
	OTPAuthURL     string `json:"otpauthUrl"`
}
