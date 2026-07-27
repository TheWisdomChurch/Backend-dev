package models

import (
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

type EventStatus string
type EventCategory string

const (
	EventStatusUpcoming  EventStatus = "upcoming"
	EventStatusHappening EventStatus = "happening"
	EventStatusPast      EventStatus = "past"
)

const (
	EventCategoryOutreach   EventCategory = "Outreach"
	EventCategoryConference EventCategory = "Conference"
	EventCategoryWorkshop   EventCategory = "Workshop"
	EventCategoryPrayer     EventCategory = "Prayer"
	EventCategoryRevival    EventCategory = "Revival"
	EventCategorySummit     EventCategory = "Summit"
)

type Event struct {
	ID               string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Title            string `gorm:"type:varchar(200);not null" json:"title" binding:"required"`
	ShortDescription string `gorm:"type:varchar(255);not null" json:"shortDescription" binding:"required"`
	Description      string `gorm:"type:text;not null" json:"description" binding:"required"`

	Date string `gorm:"type:varchar(20);not null" json:"date" binding:"required,date_ymd"`
	// EventDate is the native, indexed date used by analytics. Date remains in
	// the API temporarily for backward compatibility.
	EventDate *time.Time `gorm:"type:date;index" json:"-"`
	Time      string     `gorm:"type:varchar(50);not null" json:"time" binding:"required"`
	Location  string     `gorm:"type:varchar(255);not null" json:"location" binding:"required"`

	Category   EventCategory `gorm:"type:varchar(30);not null" json:"category" binding:"required,oneof=Outreach Conference Workshop Prayer Revival Summit"`
	Status     EventStatus   `gorm:"type:varchar(20);not null" json:"status" binding:"omitempty,oneof=upcoming happening past"`
	IsFeatured bool          `gorm:"not null;default:false" json:"isFeatured"`
	IsApproved bool          `gorm:"not null;default:false" json:"isApproved"`

	ApprovedByID    *string    `gorm:"type:uuid" json:"approvedById,omitempty"`
	ApprovedByName  *string    `gorm:"type:varchar(120)" json:"approvedByName,omitempty"`
	ApprovedByEmail *string    `gorm:"type:varchar(255)" json:"approvedByEmail,omitempty"`
	ApprovedAt      *time.Time `json:"approvedAt,omitempty"`

	Tags         pq.StringArray `gorm:"type:text[];default:'{}'" json:"tags"`
	RegisterLink *string        `gorm:"type:text" json:"registerLink,omitempty"`
	Speaker      *string        `gorm:"type:varchar(120)" json:"speaker,omitempty"`
	ContactPhone *string        `gorm:"type:varchar(40)" json:"contactPhone,omitempty"`

	// CDN URLs
	Image       *string `gorm:"type:text" json:"image,omitempty"`
	BannerImage *string `gorm:"type:text" json:"bannerImage,omitempty"`

	// Optional storage object keys (DB columns required)
	ImageKey       *string `gorm:"column:image_key;type:text" json:"-"`
	BannerImageKey *string `gorm:"column:banner_image_key;type:text" json:"-"`

	Attendees int `gorm:"not null;default:0" json:"attendees"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (e *Event) BeforeSave(_ *gorm.DB) error {
	parsed, err := time.Parse("2006-01-02", e.Date)
	if err != nil {
		return fmt.Errorf("invalid event date: %w", err)
	}
	e.EventDate = &parsed
	return nil
}
