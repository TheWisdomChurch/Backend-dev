// internal/models/form.go
package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type FormStatus string

const (
	FormStatusDraft     FormStatus = "draft"
	FormStatusPublished FormStatus = "published"
	FormStatusInvalid   FormStatus = "invalid"
)

type Form struct {
	ID string `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`

	Title       string  `gorm:"size:255;not null" json:"title"`
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// optional link to event (your Events table uses uuid string)
	EventID *string `gorm:"type:uuid;index" json:"eventId,omitempty"`

	// slug generated on publish
	Slug *string `gorm:"size:255;uniqueIndex" json:"slug,omitempty"`

	IsPublished bool       `gorm:"not null;default:false" json:"isPublished"`
	Status      FormStatus `gorm:"size:20;not null;default:'draft'" json:"status"`
	// Stored server-side only; used to authorize the public report link.
	ReportAccessToken *string `gorm:"size:160;uniqueIndex" json:"-"`

	PublishedAt *time.Time `json:"publishedAt,omitempty"`

	// JSON settings: { capacity?: number, closesAt?: string, successMessage?: string }
	Settings datatypes.JSON `gorm:"type:jsonb" json:"settings,omitempty"`

	Fields []FormField `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"fields"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Computed/derived fields (not persisted)
	PublicURL       *string    `gorm:"-" json:"publicUrl,omitempty"`
	SubmissionCount int64      `gorm:"-" json:"submissionCount"`
	ArchivedAt      *time.Time `gorm:"-" json:"archivedAt,omitempty"`
}

type FormSlugAlias struct {
	ID        string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FormID    string    `gorm:"type:uuid;not null;index" json:"formId"`
	Slug      string    `gorm:"size:255;not null;uniqueIndex" json:"slug"`
	CreatedAt time.Time `json:"createdAt"`
}

func (FormSlugAlias) TableName() string { return "form_slug_aliases" }

func (Form) TableName() string {
	return "forms"
}
