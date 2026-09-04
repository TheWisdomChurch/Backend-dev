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

	// ContentJSON is the structured content behind a form response email —
	// heading, message, CTA, calendar/resource blocks, etc. When present,
	// HTMLBody/TextBody are generated from it by
	// internal/email.RenderFormEmailContent (the single place that owns this
	// design) rather than hand-built by a caller. It round-trips as a plain
	// JSON object over the API: datatypes.JSON marshals/unmarshals its raw
	// bytes verbatim, so the admin portal reads and writes it as
	// FormEmailContent with no HTML-comment-embedding trick needed.
	ContentJSON datatypes.JSON `gorm:"type:jsonb" json:"content,omitempty"`

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

// FormEmailContent is the structured content behind one form's response
// email — the *only* shape the admin portal edits. RenderFormEmailContent
// (internal/email) is the *only* place that turns it into HTML, built from
// the same shell/component helpers as every other outbound email. There must
// never be a second hand-written copy of this rendering logic anywhere else.
type FormEmailContent struct {
	Preheader      string                  `json:"preheader,omitempty"`
	Eyebrow        string                  `json:"eyebrow,omitempty"`
	Heading        string                  `json:"heading,omitempty"`
	Message        string                  `json:"message,omitempty"`
	MessageHTML    string                  `json:"messageHtml,omitempty"`
	ImageURL       string                  `json:"imageUrl,omitempty"`
	CTALabel       string                  `json:"ctaLabel,omitempty"`
	CTAURL         string                  `json:"ctaUrl,omitempty"`
	CalendarLabel  string                  `json:"calendarLabel,omitempty"`
	CalendarURL    string                  `json:"calendarUrl,omitempty"`
	CalendarEvent  *FormEmailCalendarEvent `json:"calendarEvent,omitempty"`
	ResourceLinks  []FormEmailResourceLink `json:"resourceLinks,omitempty"`
	SpotlightLabel string                  `json:"spotlightLabel,omitempty"`
	SpotlightText  string                  `json:"spotlightText,omitempty"`
	// FooterNote is form-specific context (e.g. "This confirms your
	// children's ministry registration"), rendered above the standard
	// footer. Social links are not configurable per form — they always come
	// from the shared church branding (internal/email.Branding.Social) via
	// renderFooterBlock, the same as every other outbound email.
	FooterNote              string `json:"footerNote,omitempty"`
	IncludeRegistrationCode bool   `json:"includeRegistrationCode,omitempty"`
	IncludeCalendarOptIn    bool   `json:"includeCalendarOptIn,omitempty"`
}

type FormEmailCalendarEvent struct {
	Title       string `json:"title"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	TimeZone    string `json:"timeZone,omitempty"`
}

type FormEmailResourceLink struct {
	Label       string `json:"label"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"` // flyer|document|guide|schedule|resource
}

type CreateEmailTemplateRequest struct {
	TemplateKey string  `json:"templateKey" binding:"required"`
	OwnerType   *string `json:"ownerType,omitempty"`
	OwnerID     *string `json:"ownerId,omitempty"`
	Subject     *string `json:"subject,omitempty"`
	// HTMLBody/TextBody are optional when Content is set: the service
	// renders them from Content via internal/email.RenderFormEmailContent.
	// Provide HTMLBody directly only for a hand-authored one-off template.
	HTMLBody string               `json:"htmlBody,omitempty"`
	TextBody *string              `json:"textBody,omitempty"`
	Content  *FormEmailContent    `json:"content,omitempty"`
	Status   *EmailTemplateStatus `json:"status,omitempty"`
	Version  *int                 `json:"version,omitempty"`
	Activate bool                 `json:"activate,omitempty"`
}

type UpdateEmailTemplateRequest struct {
	TemplateKey *string              `json:"templateKey,omitempty"`
	OwnerType   *string              `json:"ownerType,omitempty"`
	OwnerID     *string              `json:"ownerId,omitempty"`
	Subject     *string              `json:"subject,omitempty"`
	HTMLBody    *string              `json:"htmlBody,omitempty"`
	TextBody    *string              `json:"textBody,omitempty"`
	Content     *FormEmailContent    `json:"content,omitempty"`
	Status      *EmailTemplateStatus `json:"status,omitempty"`
	Version     *int                 `json:"version,omitempty"`
	Activate    *bool                `json:"activate,omitempty"`
}
