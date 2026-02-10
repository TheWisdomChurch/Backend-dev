package models

type EmailTemplateType string

const (
	EmailTemplateRegistration EmailTemplateType = "registration"
	EmailTemplateBirthday     EmailTemplateType = "birthday"
	EmailTemplateOTP          EmailTemplateType = "otp"
)

type SendTemplateEmailRequest struct {
	Template       EmailTemplateType `json:"template" binding:"required,oneof=registration birthday otp"`
	Email          string            `json:"email" binding:"required,email"`
	RecipientName  string            `json:"recipientName,omitempty"`
	ActionURL      string            `json:"actionUrl,omitempty"`
	OTPCode        string            `json:"otpCode,omitempty"`
	OTPExpiresAt   string            `json:"otpExpiresAt,omitempty"`
	BirthdayDate   string            `json:"birthdayDate,omitempty"`
	CustomMessage  string            `json:"customMessage,omitempty"`
	TemplateReason string            `json:"templateReason,omitempty"`
	HeroImageURL   string            `json:"heroImageUrl,omitempty"`
}

type SendTemplateEmailResponse struct {
	Email    string `json:"email"`
	Template string `json:"template"`
	SentAt   string `json:"sentAt"`
}
