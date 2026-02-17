package models

import "time"

// FormCalendarReminder stores one-click calendar opt-in + reminder metadata
// for a form submission that is tied to an event date.
type FormCalendarReminder struct {
	ID string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`

	FormID       string `gorm:"type:uuid;not null;index" json:"formId"`
	SubmissionID string `gorm:"type:uuid;not null;uniqueIndex" json:"submissionId"`

	Slug  string `gorm:"type:varchar(255);not null;index" json:"slug"`
	Email string `gorm:"type:varchar(255);not null;index" json:"email"`

	RecipientName    *string `gorm:"type:varchar(255)" json:"recipientName,omitempty"`
	RegistrationCode *string `gorm:"type:varchar(64)" json:"registrationCode,omitempty"`

	EventTitle    string  `gorm:"type:varchar(255);not null" json:"eventTitle"`
	EventLocation *string `gorm:"type:varchar(255)" json:"eventLocation,omitempty"`
	EventDate     string  `gorm:"type:varchar(20);not null" json:"eventDate"`
	EventTime     string  `gorm:"type:varchar(64);not null" json:"eventTime"`

	EventStartsAt time.Time  `gorm:"index;not null" json:"eventStartsAt"`
	EventEndsAt   *time.Time `gorm:"index" json:"eventEndsAt,omitempty"`

	CalendarToken string `gorm:"type:varchar(120);not null;uniqueIndex" json:"-"`

	OptedInAt      *time.Time `gorm:"index" json:"optedInAt,omitempty"`
	ReminderSentAt *time.Time `gorm:"index" json:"reminderSentAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (FormCalendarReminder) TableName() string {
	return "form_calendar_reminders"
}

type FormCalendarPayload struct {
	EventTitle    string `json:"eventTitle"`
	EventDate     string `json:"eventDate"`
	EventTime     string `json:"eventTime"`
	EventLocation string `json:"eventLocation,omitempty"`
	GoogleURL     string `json:"googleUrl"`
	ICSURL        string `json:"icsUrl"`
}
