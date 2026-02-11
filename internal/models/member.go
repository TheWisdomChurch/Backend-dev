package models

import "time"

type Member struct {
	ID        string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName string `gorm:"size:100;not null" json:"firstName"`
	LastName  string `gorm:"size:100;not null" json:"lastName"`
	Email     string `gorm:"size:255;uniqueIndex;not null" json:"email"`
	Phone     string `gorm:"size:50" json:"phone,omitempty"`
	IsActive  bool   `gorm:"not null;default:true" json:"isActive"`

	// Birthday components (MM/DD). Year is not stored.
	BirthdayMonth *int `gorm:"type:smallint" json:"birthdayMonth,omitempty"`
	BirthdayDay   *int `gorm:"type:smallint" json:"birthdayDay,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (Member) TableName() string {
	return "members"
}

type CreateMemberRequest struct {
	FirstName string  `json:"firstName" binding:"required"`
	LastName  string  `json:"lastName" binding:"required"`
	Email     string  `json:"email" binding:"required,email"`
	Phone     *string `json:"phone,omitempty"`
	IsActive  *bool   `json:"isActive,omitempty"`

	// birthday in MM/DD (month/day) components
	BirthdayMonth *int `json:"birthdayMonth,omitempty"`
	BirthdayDay   *int `json:"birthdayDay,omitempty"`
	// birthday in DD/MM format (day/month)
	Birthday *string `json:"birthday,omitempty"`
}

type UpdateMemberRequest struct {
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
	Email     *string `json:"email,omitempty" binding:"omitempty,email"`
	Phone     *string `json:"phone,omitempty"`
	IsActive  *bool   `json:"isActive,omitempty"`

	BirthdayMonth *int    `json:"birthdayMonth,omitempty"`
	BirthdayDay   *int    `json:"birthdayDay,omitempty"`
	Birthday      *string `json:"birthday,omitempty"`
}

type SendMemberEmailRequest struct {
	Subject    string  `json:"subject" binding:"required"`
	Message    string  `json:"message" binding:"required"`
	EventID    *string `json:"eventId,omitempty"`
	OnlyActive *bool   `json:"onlyActive,omitempty"`
}

type BulkEmailResult struct {
	Targeted int `json:"targeted"`
	Sent     int `json:"sent"`
	Skipped  int `json:"skipped"`
}
