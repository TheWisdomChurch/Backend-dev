package models

import (
	"time"

	"gorm.io/datatypes"
)

type SiteContent struct {
	ID        string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Key       string         `gorm:"size:120;not null;uniqueIndex" json:"key"`
	Payload   datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
	UpdatedBy *string        `gorm:"size:255" json:"updatedBy,omitempty"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (SiteContent) TableName() string {
	return "site_contents"
}
