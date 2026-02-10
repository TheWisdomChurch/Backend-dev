package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type EmailTemplateStatus string

const (
	EmailTemplateDraft    EmailTemplateStatus = "draft"
	EmailTemplateActive   EmailTemplateStatus = "active"
	EmailTemplateArchived EmailTemplateStatus = "archived"
)

type EmailTemplate struct {
	ID string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`

	TemplateKey string  `gorm:"size:255;not null;index" json:"templateKey"`
	OwnerType   *string `gorm:"size:50;index" json:"ownerType,omitempty"`
	OwnerID     *string `gorm:"type:uuid;index" json:"ownerId,omitempty"`

	Subject  *string `gorm:"size:255" json:"subject,omitempty"`
	HTMLBody string  `gorm:"type:text;not null" json:"htmlBody"`
	TextBody *string `gorm:"type:text" json:"textBody,omitempty"`

	Status   EmailTemplateStatus `gorm:"size:20;not null;default:'draft'" json:"status"`
	Version  int                 `gorm:"not null;default:1" json:"version"`
	IsActive bool                `gorm:"not null;default:false" json:"isActive"`

	Metadata    datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedByID *string        `gorm:"type:uuid" json:"createdById,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (EmailTemplate) TableName() string {
	return "email_templates"
}

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

type CreateEmailTemplateRequest struct {
	TemplateKey string               `json:"templateKey" binding:"required"`
	OwnerType   *string              `json:"ownerType,omitempty"`
	OwnerID     *string              `json:"ownerId,omitempty"`
	Subject     *string              `json:"subject,omitempty"`
	HTMLBody    string               `json:"htmlBody" binding:"required"`
	TextBody    *string              `json:"textBody,omitempty"`
	Status      *EmailTemplateStatus `json:"status,omitempty"`
	Version     *int                 `json:"version,omitempty"`
	Activate    bool                 `json:"activate,omitempty"`
}

type UpdateEmailTemplateRequest struct {
	TemplateKey *string              `json:"templateKey,omitempty"`
	OwnerType   *string              `json:"ownerType,omitempty"`
	OwnerID     *string              `json:"ownerId,omitempty"`
	Subject     *string              `json:"subject,omitempty"`
	HTMLBody    *string              `json:"htmlBody,omitempty"`
	TextBody    *string              `json:"textBody,omitempty"`
	Status      *EmailTemplateStatus `json:"status,omitempty"`
	Version     *int                 `json:"version,omitempty"`
	Activate    *bool                `json:"activate,omitempty"`
}
