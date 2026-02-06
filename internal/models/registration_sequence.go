package models

import "time"

type RegistrationSequence struct {
	Prefix     string    `gorm:"type:varchar(20);primaryKey" json:"prefix"`
	LastNumber int       `gorm:"not null;default:0" json:"lastNumber"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (RegistrationSequence) TableName() string {
	return "registration_sequences"
}
