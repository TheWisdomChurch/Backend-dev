package models

import (
    "time"

    "github.com/google/uuid"
)

type Testimonial struct {
    ID        uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    FirstName string    `json:"firstName" gorm:"column:first_name;not null" binding:"required"`
    LastName  string    `json:"lastName" gorm:"column:last_name;not null" binding:"required"`
    FullName  string    `json:"fullName" gorm:"column:full_name;not null"`
    Role      string    `json:"role" gorm:"column:role"`
    Image     string    `json:"image" gorm:"column:image"`
    Testimony string    `json:"testimony" gorm:"column:testimony;not null" binding:"required"`
    Rating    int       `json:"rating" gorm:"column:rating;default:5" binding:"min=1,max=5"`
    Anonymous bool      `json:"anonymous" gorm:"column:anonymous;default:false"`
    Approved  bool      `json:"approved" gorm:"column:approved;default:false"`
    CreatedAt time.Time `json:"date" gorm:"column:created_at;autoCreateTime"`
    UpdatedAt time.Time `json:"-" gorm:"column:updated_at;autoUpdateTime"`
}

type CreateTestimonialRequest struct {
    FirstName string `json:"firstName" binding:"required"`
    LastName  string `json:"lastName" binding:"required"`
    Role      string `json:"role"`
    Image     string `json:"image"`
    Testimony string `json:"testimony" binding:"required"`
    Rating    int    `json:"rating" binding:"min=1,max=5"`
    Anonymous bool   `json:"anonymous"`
}

type UpdateTestimonialRequest struct {
    FirstName *string `json:"firstName"`
    LastName  *string `json:"lastName"`
    Role      *string `json:"role"`
    Image     *string `json:"image"`
    Testimony *string `json:"testimony"`
    Rating    *int    `json:"rating" binding:"omitempty,min=1,max=5"`
    Anonymous *bool   `json:"anonymous"`
    Approved  *bool   `json:"approved"`
}

func (Testimonial) TableName() string {
    return "testimonials"
}