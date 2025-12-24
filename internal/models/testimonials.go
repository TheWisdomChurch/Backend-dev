package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Testimonial struct {
	ID          uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	FirstName   string         `json:"firstName" gorm:"column:first_name;type:varchar(100);not null" binding:"required"`
	LastName    string         `json:"lastName" gorm:"column:last_name;type:varchar(100);not null" binding:"required"`
	FullName    string         `json:"fullName" gorm:"->;column:full_name;type:varchar(200);generatedAlwaysAs:(first_name || ' ' || last_name) stored"`
	ImageURL    *string        `json:"imageUrl,omitempty" gorm:"column:image_url;type:varchar(500)"` // Pointer for NULL
	Testimony   string         `json:"testimony" gorm:"column:testimony;type:text;not null" binding:"required"`
	IsAnonymous bool           `json:"isAnonymous" gorm:"column:is_anonymous;default:false"`
	IsApproved  bool           `json:"isApproved" gorm:"column:is_approved;default:false"`
	CreatedAt   time.Time      `json:"createdAt" gorm:"column:created_at;autoCreateTime"` // Changed from "date" to "createdAt"
	UpdatedAt   time.Time      `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type CreateTestimonialRequest struct {
	FirstName   string  `json:"firstName" binding:"required"`
	LastName    string  `json:"lastName" binding:"required"`
	ImageURL    *string `json:"imageUrl,omitempty"` // Pointer for optional field
	Testimony   string  `json:"testimony" binding:"required"`
	IsAnonymous bool    `json:"isAnonymous"`
}

type UpdateTestimonialRequest struct {
	FirstName   *string `json:"firstName"`
	LastName    *string `json:"lastName"`
	ImageURL    *string `json:"imageUrl,omitempty"` // Pointer for optional field
	Testimony   *string `json:"testimony"`
	IsAnonymous *bool   `json:"isAnonymous"`
	IsApproved  *bool   `json:"isApproved"`
}

func (Testimonial) TableName() string {
	return "testimonials"
}
