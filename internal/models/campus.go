package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Campus struct {
	ID        string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name      string         `gorm:"not null;uniqueIndex;size:150" json:"name"`
	Address   string         `gorm:"size:300" json:"address"`
	City      string         `gorm:"size:100" json:"city"`
	PhoneEnc  *string        `gorm:"column:phone_enc;type:text" json:"-"`
	TimeZone  string         `gorm:"size:64;default:'UTC'" json:"time_zone"`
	IsActive  bool           `gorm:"default:true;not null" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (c *Campus) BeforeCreate(_ *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	return nil
}
