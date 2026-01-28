// internal/models/user.go
package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID                string         `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	FirstName         string         `gorm:"size:100;not null" json:"first_name"`
	LastName          string         `gorm:"size:100;not null" json:"last_name"`
	Email             string         `gorm:"size:255;uniqueIndex;not null" json:"email"`
	Password          string         `gorm:"size:255;not null" json:"-"` // Hidden from JSON
	Role              string         `gorm:"size:50;not null;default:'admin'" json:"role"`
	IsActive          bool           `gorm:"default:true;not null" json:"is_active"` // ADD THIS LINE
	AdminApproved     bool           `gorm:"default:true;not null" json:"admin_approved"`
	FailedLoginCount  int            `gorm:"default:0;not null" json:"-"`
	LastFailedLoginAt *time.Time     `json:"-"`
	LastLoginAt       *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt         time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// Optional: Add TableName method for consistency
func (User) TableName() string {
	return "users"
}
