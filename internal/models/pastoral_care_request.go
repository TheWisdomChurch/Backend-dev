package models

import (
	"time"

	"gorm.io/datatypes"
)

type PastoralCareRequest struct {
	ID            string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Title         string         `gorm:"size:40;not null" json:"title"`
	FirstName     string         `gorm:"size:120;not null" json:"firstName"`
	LastName      string         `gorm:"size:120;not null" json:"lastName"`
	Phone         string         `gorm:"size:60;not null" json:"phone"`
	Email         string         `gorm:"size:255;not null" json:"email"`
	Address       string         `gorm:"size:500;not null" json:"address"`
	EventDate     string         `gorm:"size:20;not null" json:"eventDate"`
	EventType     string         `gorm:"size:120;not null" json:"eventType"`
	ChurchRole    string         `gorm:"size:120;not null" json:"churchRole"`
	CustomRole    string         `gorm:"size:120" json:"customRole,omitempty"`
	Comments      string         `gorm:"type:text" json:"comments,omitempty"`
	SourceChannel string         `gorm:"size:120;not null;default:'frontend:web:pastoral-care'" json:"sourceChannel"`
	Metadata      datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (PastoralCareRequest) TableName() string {
	return "pastoral_care_requests"
}
