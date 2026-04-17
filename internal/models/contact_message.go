package models

import "time"

type ContactMessage struct {
	ID            string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName     string         `gorm:"size:120;not null" json:"firstName"`
	LastName      string         `gorm:"size:120;not null" json:"lastName"`
	Email         string         `gorm:"size:255;index;not null" json:"email"`
	Phone         string         `gorm:"size:60" json:"phone,omitempty"`
	Topic         string         `gorm:"size:120;index" json:"topic,omitempty"`
	Message       string         `gorm:"type:text;not null" json:"message"`
	SourceChannel string         `gorm:"size:120;not null;default:'frontend:web:contact'" json:"sourceChannel"`
	Metadata      map[string]any `gorm:"serializer:json" json:"metadata,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

func (ContactMessage) TableName() string {
	return "contact_messages"
}
