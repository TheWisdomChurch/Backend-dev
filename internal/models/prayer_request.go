package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PrayerRequest stores pastoral prayer requests.
// RequestEnc and NotesEnc are ALWAYS AES-256-GCM encrypted — never plaintext.
// The service layer handles encrypt-on-write and decrypt-on-read.
// Every admin read must be recorded in the security audit log.
type PrayerRequest struct {
	ID          string         `gorm:"type:uuid;primaryKey" json:"id"`
	MemberID    *string        `gorm:"type:uuid;index" json:"member_id,omitempty"`
	FirstName   string         `gorm:"size:100;not null" json:"first_name"`
	LastName    string         `gorm:"size:100;not null" json:"last_name"`
	Email       string         `gorm:"size:255;index" json:"email,omitempty"`
	RequestEnc  string         `gorm:"type:text;not null" json:"-"` // AES-256-GCM encrypted
	Category    string         `gorm:"size:100;index" json:"category,omitempty"`
	IsAnonymous bool           `gorm:"default:false;not null" json:"is_anonymous"`
	Status      string         `gorm:"size:50;default:'pending';index" json:"status"` // pending, praying, answered, closed
	AssignedTo  *string        `gorm:"type:uuid;index" json:"assigned_to,omitempty"`
	NotesEnc    *string        `gorm:"type:text" json:"-"` // AES-256-GCM encrypted pastoral notes
	CreatedAt   time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Decrypted fields — populated by service layer, never stored.
	Request string  `gorm:"-" json:"request,omitempty"`
	Notes   *string `gorm:"-" json:"notes,omitempty"`
}

func (p *PrayerRequest) BeforeCreate(_ *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	return nil
}
