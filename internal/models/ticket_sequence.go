package models

import "time"

type TicketSequence struct {
	Prefix     string    `gorm:"type:varchar(40);primaryKey" json:"prefix"`
	LastNumber int       `gorm:"not null;default:0" json:"lastNumber"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (TicketSequence) TableName() string {
	return "ticket_sequences"
}
