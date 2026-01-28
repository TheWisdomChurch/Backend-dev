// internal/models/otp.go
package models

import (
	"time"

	"gorm.io/gorm"
)

type OTP struct {
	ID       string `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	Email    string `gorm:"size:255;index;not null" json:"email"`
	Purpose  string `gorm:"size:120;index" json:"purpose,omitempty"`
	CodeHash string `gorm:"size:64;not null" json:"-"`
	CodeSalt string `gorm:"size:32;not null" json:"-"`

	ExpiresAt time.Time  `gorm:"index;not null" json:"expiresAt"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (OTP) TableName() string {
	return "otps"
}

type SendOTPRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Purpose     string `json:"purpose,omitempty"`
	ActionURL   string `json:"actionUrl,omitempty"`
	ActionLabel string `json:"actionLabel,omitempty"`
}

type VerifyOTPRequest struct {
	Email   string `json:"email" binding:"required,email"`
	Code    string `json:"code" binding:"required"`
	Purpose string `json:"purpose,omitempty"`
}

type SendOTPResponse struct {
	ExpiresAt time.Time `json:"expiresAt"`
	Purpose   string    `json:"purpose,omitempty"`
	ActionURL string    `json:"actionUrl,omitempty"`
}

type VerifyOTPResponse struct {
	Verified bool `json:"verified"`
}
