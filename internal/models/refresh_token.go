package models

import "time"

// RefreshToken implements the token family pattern for secure refresh token rotation.
//
// When a refresh token is used, it is atomically revoked and a new one is issued
// in the same family. If a revoked token is ever presented again (replay attack),
// the entire family is revoked and the user is logged out everywhere.
type RefreshToken struct {
	ID        string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"-"`
	UserID    string     `gorm:"type:uuid;not null;index"                       json:"-"`
	FamilyID  string     `gorm:"type:uuid;not null;index"                       json:"-"`
	TokenHash string     `gorm:"type:char(64);not null;uniqueIndex"              json:"-"` // SHA-256 of raw token
	DeviceID  string     `gorm:"size:255"                                        json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index"                                 json:"-"`
	RevokedAt *time.Time `                                                      json:"-"`
	CreatedAt time.Time  `gorm:"autoCreateTime"                                 json:"-"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
