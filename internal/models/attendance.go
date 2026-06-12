package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceType struct {
	ID        string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name      string         `gorm:"not null;size:150" json:"name"`
	CampusID  *string        `gorm:"type:uuid;index" json:"campus_id,omitempty"`
	IsActive  bool           `gorm:"default:true;not null" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (s *ServiceType) BeforeCreate(_ *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	return nil
}

type AttendanceSession struct {
	ID            string         `gorm:"type:uuid;primaryKey" json:"id"`
	CampusID      *string        `gorm:"type:uuid;index" json:"campus_id,omitempty"`
	ServiceTypeID string         `gorm:"type:uuid;not null;index" json:"service_type_id"`
	ServiceType   *ServiceType   `gorm:"foreignKey:ServiceTypeID" json:"service_type,omitempty"`
	Date          time.Time      `gorm:"not null;index" json:"date"`
	HeadCount     int            `gorm:"default:0" json:"head_count"`
	Notes         string         `gorm:"type:text" json:"notes,omitempty"`
	CreatedByID   *string        `gorm:"type:uuid" json:"created_by_id,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (a *AttendanceSession) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	return nil
}

// AttendanceRecord tracks individual member check-ins per session.
type AttendanceRecord struct {
	ID           string         `gorm:"type:uuid;primaryKey" json:"id"`
	SessionID    string         `gorm:"type:uuid;not null;index" json:"session_id"`
	MemberID     *string        `gorm:"type:uuid;index" json:"member_id,omitempty"`
	GuestName    string         `gorm:"size:200" json:"guest_name,omitempty"`
	CheckedInAt  time.Time      `gorm:"not null" json:"checked_in_at"`
	CheckedInVia string         `gorm:"size:50;default:'manual'" json:"checked_in_via"` // manual, qr, app
	CreatedAt    time.Time      `json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (a *AttendanceRecord) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.CheckedInAt.IsZero() {
		a.CheckedInAt = time.Now().UTC()
	}
	return nil
}
