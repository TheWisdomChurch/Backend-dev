// internal/models/form.go
package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Form struct {
	ID string `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`

	Title       string  `gorm:"size:255;not null" json:"title"`
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// optional link to event (your Events table uses uuid string)
	EventID *string `gorm:"type:uuid;index" json:"eventId,omitempty"`

	// slug generated on publish
	Slug *string `gorm:"size:255;uniqueIndex" json:"slug,omitempty"`

	IsPublished bool `gorm:"not null;default:false" json:"isPublished"`

	// JSON settings: { capacity?: number, closesAt?: string, successMessage?: string }
	Settings datatypes.JSON `gorm:"type:jsonb" json:"settings,omitempty"`

	Fields []FormField `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"fields"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Form) TableName() string {
	return "forms"
}
