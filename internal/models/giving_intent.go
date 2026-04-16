package models

import (
	"time"

	"gorm.io/datatypes"
)

type GivingIntent struct {
	ID            string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Title         string         `gorm:"size:200;not null" json:"title"`
	Description   string         `gorm:"type:text" json:"description,omitempty"`
	SourceChannel string         `gorm:"size:120;not null;default:'frontend:web:online-giving'" json:"sourceChannel"`
	Metadata      datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (GivingIntent) TableName() string {
	return "giving_intents"
}
