package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Ministry struct {
	ID          string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string         `gorm:"not null;size:150" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	CampusID    *string        `gorm:"type:uuid;index" json:"campus_id,omitempty"`
	LeaderID    *string        `gorm:"type:uuid;index" json:"leader_id,omitempty"`
	Category    string         `gorm:"size:100;index" json:"category,omitempty"` // worship, children, media, ushering, etc.
	IsActive    bool           `gorm:"default:true;not null" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (m *Ministry) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return nil
}

type MinistryMember struct {
	ID         string         `gorm:"type:uuid;primaryKey" json:"id"`
	MinistryID string         `gorm:"type:uuid;not null;uniqueIndex:idx_ministry_member" json:"ministry_id"`
	MemberID   string         `gorm:"type:uuid;not null;uniqueIndex:idx_ministry_member;index" json:"member_id"`
	Role       string         `gorm:"size:50;default:'member'" json:"role"`
	JoinedAt   time.Time      `gorm:"not null" json:"joined_at"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (m *MinistryMember) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = time.Now().UTC()
	}
	return nil
}
