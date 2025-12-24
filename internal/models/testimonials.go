package models

import (
    "time"

    "github.com/google/uuid"
)

type Testimonial struct {
    ID           uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
    FirstName    string    `json:"firstName" gorm:"column:first_name;not null" binding:"required"`
    LastName     string    `json:"lastName" gorm:"column:last_name;not null" binding:"required"`
    FullName     string    `json:"fullName" gorm:"column:full_name;generated"`
    ImageURL     string    `json:"imageUrl" gorm:"column:image_url"`
    Testimony    string    `json:"testimony" gorm:"column:testimony;not null" binding:"required"`
    IsAnonymous  bool      `json:"isAnonymous" gorm:"column:is_anonymous;default:false"`
    IsApproved   bool      `json:"isApproved" gorm:"column:is_approved;default:false"`
    CreatedAt    time.Time `json:"date" gorm:"column:created_at;autoCreateTime"`
    UpdatedAt    time.Time `json:"-" gorm:"column:updated_at;autoUpdateTime"`
}

type CreateTestimonialRequest struct {
    FirstName   string `json:"firstName" binding:"required"`
    LastName    string `json:"lastName" binding:"required"`
    ImageURL    string `json:"imageUrl"`
    Testimony   string `json:"testimony" binding:"required"`
    IsAnonymous bool   `json:"isAnonymous"`
}

type UpdateTestimonialRequest struct {
    FirstName   *string `json:"firstName"`
    LastName    *string `json:"lastName"`
    ImageURL    *string `json:"imageUrl"`
    Testimony   *string `json:"testimony"`
    IsAnonymous *bool   `json:"isAnonymous"`
    IsApproved  *bool   `json:"isApproved"`
}

func (Testimonial) TableName() string {
    return "testimonials"
}