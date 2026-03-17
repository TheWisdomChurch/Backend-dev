package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AdminEmailAudienceSource string

const (
	AdminEmailAudienceManual AdminEmailAudienceSource = "manual"
	AdminEmailAudienceForms  AdminEmailAudienceSource = "forms"
	AdminEmailAudienceMixed  AdminEmailAudienceSource = "mixed"
)

type AdminEmailDeliveryStatus string

const (
	AdminEmailDeliveryCompleted AdminEmailDeliveryStatus = "completed"
	AdminEmailDeliveryPartial   AdminEmailDeliveryStatus = "partial"
	AdminEmailDeliveryFailed    AdminEmailDeliveryStatus = "failed"
)

type AdminEmailSendActor struct {
	UserID string
	Email  string
	Role   string
}

type AdminEmailDelivery struct {
	ID string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`

	Subject        string                   `gorm:"type:varchar(255);not null" json:"subject"`
	TemplateSource string                   `gorm:"type:varchar(120);not null" json:"templateSource"`
	TemplateID     *string                  `gorm:"type:varchar(120)" json:"templateId,omitempty"`
	TemplateKey    *string                  `gorm:"type:varchar(255)" json:"templateKey,omitempty"`
	AudienceSource AdminEmailAudienceSource `gorm:"type:varchar(20);not null;default:'manual';index" json:"audienceSource"`

	ManualRecipients int            `gorm:"not null;default:0" json:"manualRecipients"`
	FormRecipients   int            `gorm:"not null;default:0" json:"formRecipients"`
	SourceForms      datatypes.JSON `gorm:"type:jsonb" json:"sourceForms,omitempty"`

	Status           AdminEmailDeliveryStatus `gorm:"type:varchar(20);not null;default:'completed';index" json:"status"`
	TotalRecipients  int                      `gorm:"not null;default:0" json:"totalRecipients"`
	Targeted         int                      `gorm:"not null;default:0" json:"targeted"`
	Sent             int                      `gorm:"not null;default:0" json:"sent"`
	Skipped          int                      `gorm:"not null;default:0" json:"skipped"`
	Failed           int                      `gorm:"not null;default:0" json:"failed"`
	FailedRecipients datatypes.JSON           `gorm:"type:jsonb" json:"failedRecipients,omitempty"`

	StartedAt   time.Time  `gorm:"not null;index" json:"startedAt"`
	CompletedAt *time.Time `gorm:"index" json:"completedAt,omitempty"`

	CreatedByUserID *string `gorm:"type:uuid" json:"createdByUserId,omitempty"`
	CreatedByEmail  *string `gorm:"type:varchar(255)" json:"createdByEmail,omitempty"`
	CreatedByRole   *string `gorm:"type:varchar(50)" json:"createdByRole,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (AdminEmailDelivery) TableName() string {
	return "admin_email_deliveries"
}
