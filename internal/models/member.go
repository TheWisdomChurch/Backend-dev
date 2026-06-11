package models

import "time"

type Member struct {
	ID        string  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName string  `gorm:"size:100;not null" json:"firstName"`
	LastName  string  `gorm:"size:100;not null" json:"lastName"`
	Email     string  `gorm:"size:255;not null;index:idx_members_email" json:"email"`
	Phone     *string `gorm:"column:phone;size:50" json:"phone,omitempty"`
	PhoneEnc  *string `gorm:"column:phone_enc;type:text" json:"-"` // AES-256-GCM ciphertext; decrypted into Phone by service layer
	IsActive  bool    `gorm:"not null;default:true" json:"isActive"`

	// Birthday is stored as month/day components so yearly birthday automation works without storing a birth year.
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

	BirthdayMonth *int    `json:"birthdayMonth,omitempty"`
	BirthdayDay   *int    `json:"birthdayDay,omitempty"`
	Birthday      *string `json:"birthday,omitempty"` // DD/MM
}

type UpdateMemberRequest struct {
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
	Email     *string `json:"email,omitempty" binding:"omitempty,email"`
	Phone     *string `json:"phone,omitempty"`
	IsActive  *bool   `json:"isActive,omitempty"`

	BirthdayMonth *int    `json:"birthdayMonth,omitempty"`
	BirthdayDay   *int    `json:"birthdayDay,omitempty"`
	Birthday      *string `json:"birthday,omitempty"` // DD/MM
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
