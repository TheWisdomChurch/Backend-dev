package models

import "time"

type Reel struct {
	ID        string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Title     string `gorm:"type:varchar(200);not null" json:"title" binding:"required"`
	Thumbnail string `gorm:"type:text;not null" json:"thumbnail" binding:"required"`
	VideoURL  string `gorm:"type:text;not null" json:"videoUrl" binding:"required"`
	Duration  string `gorm:"type:varchar(20);not null;default:'0:00'" json:"duration" binding:"required"`

	// optional relationship
	EventID *string `gorm:"type:uuid" json:"eventId,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
