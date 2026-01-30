package models

import (
	"time"

	"gorm.io/gorm"
)

// TrustedDevice records devices a user has used, enabling anomaly detection.
type TrustedDevice struct {
	ID         string         `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	UserID     string         `gorm:"type:uuid;index;not null" json:"user_id"`
	DeviceID   string         `gorm:"size:255;index;not null" json:"device_id"`
	Label      string         `gorm:"size:255" json:"label,omitempty"`
	LastIP     string         `gorm:"size:45" json:"last_ip,omitempty"`
	UserAgent  string         `gorm:"size:512" json:"user_agent,omitempty"`
	Trusted    bool           `gorm:"default:true;not null" json:"trusted"`
	ExpiresAt  time.Time      `json:"expires_at"`
	LastSeenAt time.Time      `gorm:"autoUpdateTime" json:"last_seen_at"`
	CreatedAt  time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}
