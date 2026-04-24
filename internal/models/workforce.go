package models

import "time"

type WorkforceStatus string

const (
	WorkforceStatusPending    WorkforceStatus = "pending"
	WorkforceStatusNew        WorkforceStatus = "new" // legacy/alias for pending
	WorkforceStatusServing    WorkforceStatus = "serving"
	WorkforceStatusNotServing WorkforceStatus = "not_serving"
)

type WorkforceMember struct {
	ID            string          `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName     string          `gorm:"size:100;not null" json:"firstName"`
	LastName      string          `gorm:"size:100;not null" json:"lastName"`
	Email         *string         `gorm:"size:255;index" json:"email,omitempty"`
	Phone         *string         `gorm:"size:50" json:"phone,omitempty"`
	Department    string          `gorm:"size:120;index;not null" json:"department"`
	SourceChannel string          `gorm:"size:120;index;not null;default:'frontend:web:workforce'" json:"sourceChannel"`
	Status        WorkforceStatus `gorm:"size:20;not null;default:'pending'" json:"status"`
	Notes         *string         `gorm:"type:text" json:"notes,omitempty"`

	// Birthday components (MM/DD). Year is not stored.
	BirthdayMonth *int `gorm:"type:smallint" json:"birthdayMonth,omitempty"`
	BirthdayDay   *int `gorm:"type:smallint" json:"birthdayDay,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (WorkforceMember) TableName() string {
	return "workforce_members"
}

type CreateWorkforceRequest struct {
	FirstName     string          `json:"firstName" binding:"required"`
	LastName      string          `json:"lastName" binding:"required"`
	Email         string          `json:"email" binding:"omitempty,email"`
	Phone         string          `json:"phone"`
	Department    string          `json:"department" binding:"required"`
	SourceChannel string          `json:"sourceChannel"`
	Status        WorkforceStatus `json:"status" binding:"omitempty,oneof=pending new serving not_serving"`
	Notes         *string         `json:"notes,omitempty"`

	// birthday in MM/DD (month/day) components
	BirthdayMonth *int `json:"birthdayMonth,omitempty"`
	BirthdayDay   *int `json:"birthdayDay,omitempty"`
	// birthday in DD/MM format (day/month)
	Birthday *string `json:"birthday,omitempty"`
}

type UpdateWorkforceRequest struct {
	FirstName  *string          `json:"firstName,omitempty"`
	LastName   *string          `json:"lastName,omitempty"`
	Email      *string          `json:"email,omitempty"`
	Phone      *string          `json:"phone,omitempty"`
	Department *string          `json:"department,omitempty"`
	Status     *WorkforceStatus `json:"status,omitempty" binding:"omitempty,oneof=pending new serving not_serving"`
	Notes      *string          `json:"notes,omitempty"`

	BirthdayMonth *int    `json:"birthdayMonth,omitempty"`
	BirthdayDay   *int    `json:"birthdayDay,omitempty"`
	Birthday      *string `json:"birthday,omitempty"`
}

type WorkforceStatsResponse struct {
	Total                int64             `json:"total"`
	ByStatus             map[string]int64  `json:"byStatus"`
	ByDepartment         map[string]int64  `json:"byDepartment"`
	BySource             map[string]int64  `json:"bySource"`
	FrontendByDepartment map[string]int64  `json:"frontendByDepartment"`
	ByDeptAndStatus      []WorkforceBucket `json:"byDeptAndStatus"`
}

type WorkforceBucket struct {
	Department string `json:"department"`
	Status     string `json:"status"`
	Count      int64  `json:"count"`
}

type BirthdayMonthCount struct {
	Month int   `json:"month"`
	Count int64 `json:"count"`
}

type BirthdayStatsResponse struct {
	Total   int64                `json:"total"`
	ByMonth []BirthdayMonthCount `json:"byMonth"`
}

type BirthdaySendResult struct {
	Targeted int `json:"targeted"`
	Sent     int `json:"sent"`
	Skipped  int `json:"skipped"`
}
