package models

import "time"

// VisitRequest is the durable lifecycle record for a planned church visit.
// It is deliberately separate from contact_messages: visits have scheduling,
// reminder, ownership, and follow-up semantics that a generic inbox cannot
// represent safely.
type VisitRequest struct {
	ID                 string     `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName          string     `gorm:"size:120;not null" json:"firstName"`
	LastName           string     `gorm:"size:120;not null" json:"lastName"`
	Email              string     `gorm:"size:255;index;not null" json:"email"`
	Phone              string     `gorm:"size:60" json:"phone,omitempty"`
	ServiceDate        time.Time  `gorm:"type:date;index;not null" json:"serviceDate"`
	ServiceAt          time.Time  `gorm:"index;not null" json:"serviceAt"`
	ServiceType        string     `gorm:"size:120;index;not null" json:"serviceType"`
	Attendance         int        `gorm:"not null;default:1" json:"attendance"`
	Notes              string     `gorm:"type:text" json:"notes,omitempty"`
	Status             string     `gorm:"size:40;index;not null;default:'new'" json:"status"`
	AssignedTo         *string    `gorm:"type:uuid;index" json:"assignedTo,omitempty"`
	NextFollowUpAt     *time.Time `gorm:"index" json:"nextFollowUpAt,omitempty"`
	FollowUpNotifiedAt *time.Time `gorm:"index" json:"followUpNotifiedAt,omitempty"`
	LastContactAt      *time.Time `json:"lastContactAt,omitempty"`
	ConfirmationSentAt *time.Time `json:"confirmationSentAt,omitempty"`
	ReminderSentAt     *time.Time `gorm:"index" json:"reminderSentAt,omitempty"`
	CheckedInAt        *time.Time `json:"checkedInAt,omitempty"`
	SourceChannel      string     `gorm:"size:120;not null;default:'frontend:web:plan-visit'" json:"sourceChannel"`
	IdempotencyKey     string     `gorm:"size:160;uniqueIndex;not null" json:"-"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func (VisitRequest) TableName() string { return "visit_requests" }
