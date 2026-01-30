package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SecurityEvent captures noteworthy auth/security related activities for auditing and alerting.
type SecurityEvent struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	UserID    *string        `gorm:"type:uuid" json:"user_id,omitempty"`
	Email     string         `gorm:"size:255;index" json:"email"`
	Type      string         `gorm:"size:100;index;not null" json:"type"`
	IP        string         `gorm:"size:45" json:"ip,omitempty"`
	UserAgent string         `gorm:"size:512" json:"user_agent,omitempty"`
	Metadata  datatypes.JSON `json:"metadata,omitempty"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
