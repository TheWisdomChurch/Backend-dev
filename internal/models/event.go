package models

import "time"

type EventStatus string
type EventCategory string

const (
	EventStatusUpcoming  EventStatus = "upcoming"
	EventStatusHappening EventStatus = "happening"
	EventStatusPast      EventStatus = "past"
)

const (
	EventCategoryOutreach    EventCategory = "Outreach"
	EventCategoryConference  EventCategory = "Conference"
	EventCategoryWorkshop    EventCategory = "Workshop"
	EventCategoryPrayer      EventCategory = "Prayer"
	EventCategoryRevival     EventCategory = "Revival"
	EventCategorySummit      EventCategory = "Summit"
)

type Event struct {
	ID               string        `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Title            string        `gorm:"type:varchar(200);not null" json:"title"`
	ShortDescription string        `gorm:"type:varchar(255);not null" json:"shortDescription"`
	Description      string        `gorm:"type:text;not null" json:"description"`

	Date     string `gorm:"type:varchar(20);not null" json:"date"` // keep string (matches frontend)
	Time     string `gorm:"type:varchar(50);not null" json:"time"`
	Location string `gorm:"type:varchar(255);not null" json:"location"`

	Category   EventCategory `gorm:"type:varchar(30);not null" json:"category"`
	Status     EventStatus   `gorm:"type:varchar(20);not null" json:"status"`
	IsFeatured bool          `gorm:"not null;default:false" json:"isFeatured"`

	Tags         []string `gorm:"type:text[];default:'{}'" json:"tags"` // Postgres array
	RegisterLink *string  `gorm:"type:text" json:"registerLink,omitempty"`
	Speaker      *string  `gorm:"type:varchar(120)" json:"speaker,omitempty"`
	ContactPhone *string  `gorm:"type:varchar(40)" json:"contactPhone,omitempty"`

	Image       *string `gorm:"type:text" json:"image,omitempty"`
	BannerImage *string `gorm:"type:text" json:"bannerImage,omitempty"`

	Attendees int `gorm:"not null;default:0" json:"attendees"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
