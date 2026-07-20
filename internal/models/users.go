// internal/models/user.go
package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID                 string         `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	FirstName          string         `gorm:"size:100;not null" json:"first_name"`
	LastName           string         `gorm:"size:100;not null" json:"last_name"`
	Email              string         `gorm:"size:255;uniqueIndex;not null" json:"email"`
	Password           string         `gorm:"size:255;not null" json:"-"` // Hidden from JSON
	FederatedProvider  *string        `gorm:"size:50" json:"federated_provider,omitempty"`
	FederatedSubject   *string        `gorm:"size:255;uniqueIndex" json:"-"`
	FederatedLinkedAt  *time.Time     `json:"federated_linked_at,omitempty"`
	PreferredMFAMethod string         `gorm:"size:30;not null;default:'email_otp'" json:"preferred_mfa_method"`
	TOTPEnabled        bool           `gorm:"default:false;not null" json:"totp_enabled"`
	TOTPSecretEnc      *string        `gorm:"type:text" json:"-"`
	TOTPPendingEnc     *string        `gorm:"type:text" json:"-"`
	Role               string         `gorm:"size:50;not null;default:'admin'" json:"role"`
	IsActive           bool           `gorm:"default:true;not null" json:"is_active"` // ADD THIS LINE
	AdminApproved      bool           `gorm:"default:true;not null" json:"admin_approved"`
	FailedLoginCount   int            `gorm:"default:0;not null" json:"-"`
	LastFailedLoginAt  *time.Time     `json:"-"`
	IsLocked           bool           `gorm:"default:false;not null" json:"-"`
	LockedUntil        *time.Time     `json:"-"`
	LastLoginAt        *time.Time     `json:"last_login_at,omitempty"`
	CreatedAt          time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

// Optional: Add TableName method for consistency
func (User) TableName() string {
	return "users"
}

// ApprovalRequesterFields builds the (id, name, email) triple approval-ticket
// creation stores as "who requested this" — nil-safe so callers can pass a
// possibly-nil *User straight through without a guard.
func (u *User) ApprovalRequesterFields() (id, name, email *string) {
	if u == nil {
		return nil, nil, nil
	}

	id = &u.ID

	fullName := strings.TrimSpace(strings.Join([]string{u.FirstName, u.LastName}, " "))
	if fullName != "" {
		name = &fullName
	}

	trimmedEmail := strings.TrimSpace(u.Email)
	if trimmedEmail != "" {
		email = &trimmedEmail
	}

	return id, name, email
}
