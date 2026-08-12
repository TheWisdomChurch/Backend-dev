package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AdminEmailScheduleStatus string

const (
	AdminEmailScheduleDraft     AdminEmailScheduleStatus = "draft"
	AdminEmailScheduleActive    AdminEmailScheduleStatus = "active"
	AdminEmailSchedulePaused    AdminEmailScheduleStatus = "paused"
	AdminEmailScheduleCompleted AdminEmailScheduleStatus = "completed"
	AdminEmailScheduleFailed    AdminEmailScheduleStatus = "failed"
)

type AdminEmailRecurrence string

const (
	AdminEmailRecurrenceOnce    AdminEmailRecurrence = "once"
	AdminEmailRecurrenceWeekly  AdminEmailRecurrence = "weekly"
	AdminEmailRecurrenceMonthly AdminEmailRecurrence = "monthly"
)

// AdminEmailSchedule stores an immutable compose payload together with its
// recurrence rule. NextRunAt is always UTC; Timezone preserves the editor's
// IANA zone so future daylight-saving changes are calculated correctly.
type AdminEmailSchedule struct {
	ID string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`

	Name                string                   `gorm:"type:varchar(160);not null" json:"name"`
	Description         string                   `gorm:"type:varchar(500);not null;default:''" json:"description"`
	Status              AdminEmailScheduleStatus `gorm:"type:varchar(20);not null;default:'draft';index" json:"status"`
	Recurrence          AdminEmailRecurrence     `gorm:"type:varchar(20);not null;index" json:"recurrence"`
	Timezone            string                   `gorm:"type:varchar(80);not null" json:"timezone"`
	SendTime            string                   `gorm:"type:char(5);not null" json:"sendTime"`
	StartDate           string                   `gorm:"type:date;not null" json:"startDate"`
	EndDate             *string                  `gorm:"type:date" json:"endDate,omitempty"`
	Weekdays            datatypes.JSON           `gorm:"type:jsonb;not null;default:'[]'" json:"weekdays"`
	MonthDays           datatypes.JSON           `gorm:"type:jsonb;not null;default:'[]'" json:"monthDays"`
	StartAt             time.Time                `gorm:"not null" json:"startAt"`
	EndAt               *time.Time               `json:"endAt,omitempty"`
	NextRunAt           *time.Time               `gorm:"index" json:"nextRunAt,omitempty"`
	LastRunAt           *time.Time               `json:"lastRunAt,omitempty"`
	PendingOccurrenceAt *time.Time               `gorm:"index" json:"-"`

	ComposePayload datatypes.JSON `gorm:"type:jsonb;not null" json:"-"`
	Subject        string         `gorm:"type:varchar(255);not null" json:"subject"`
	AudienceLabel  string         `gorm:"type:varchar(255);not null;default:''" json:"audienceLabel"`

	RunCount          int        `gorm:"not null;default:0" json:"runCount"`
	ConsecutiveErrors int        `gorm:"not null;default:0" json:"consecutiveErrors"`
	LastError         *string    `gorm:"type:text" json:"lastError,omitempty"`
	ClaimedAt         *time.Time `gorm:"index" json:"-"`
	ClaimedBy         *string    `gorm:"type:varchar(120)" json:"-"`
	Version           int        `gorm:"not null;default:1" json:"version"`

	CreatedByUserID *string `gorm:"type:uuid" json:"createdByUserId,omitempty"`
	CreatedByEmail  *string `gorm:"type:varchar(255)" json:"createdByEmail,omitempty"`
	CreatedByRole   *string `gorm:"type:varchar(50)" json:"createdByRole,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (AdminEmailSchedule) TableName() string { return "admin_email_schedules" }

type AdminEmailScheduleRun struct {
	ID           string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	ScheduleID   string     `gorm:"type:uuid;not null;index;uniqueIndex:ux_schedule_occurrence" json:"scheduleId"`
	ScheduledFor time.Time  `gorm:"not null;index;uniqueIndex:ux_schedule_occurrence" json:"scheduledFor"`
	Status       string     `gorm:"type:varchar(20);not null;index" json:"status"`
	Attempt      int        `gorm:"not null;default:1" json:"attempt"`
	DeliveryID   *string    `gorm:"type:uuid" json:"deliveryId,omitempty"`
	Sent         int        `gorm:"not null;default:0" json:"sent"`
	Failed       int        `gorm:"not null;default:0" json:"failed"`
	Error        *string    `gorm:"type:text" json:"error,omitempty"`
	StartedAt    time.Time  `gorm:"not null" json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"createdAt"`
}

func (AdminEmailScheduleRun) TableName() string { return "admin_email_schedule_runs" }
