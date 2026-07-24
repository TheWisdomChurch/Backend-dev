package models

import "time"

type WorkforceStatus string

const (
	WorkforceStatusPending    WorkforceStatus = "pending"
	WorkforceStatusNew        WorkforceStatus = "new"
	WorkforceStatusServing    WorkforceStatus = "serving"
	WorkforceStatusNotServing WorkforceStatus = "not_serving"
)

type WorkforceMember struct {
	ID            string          `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName     string          `gorm:"size:100;not null" json:"firstName"`
	LastName      string          `gorm:"size:100;not null" json:"lastName"`
	Email         *string         `gorm:"size:255;index:idx_workforce_members_email" json:"email,omitempty"`
	Phone         *string         `gorm:"size:50" json:"phone,omitempty"`
	Department    string          `gorm:"size:120;not null;index:idx_workforce_members_department" json:"department"`
	SourceChannel string          `gorm:"size:120;not null;default:'frontend:web:workforce';index:idx_workforce_members_source_channel" json:"sourceChannel"`
	Status        WorkforceStatus `gorm:"size:20;not null;default:'pending';index:idx_workforce_members_status" json:"status"`
	Notes         *string         `gorm:"type:text" json:"notes,omitempty"`

	// Birthday/anniversary are stored as month/day components so yearly
	// greeting automation works without storing a birth/wedding year.
	BirthdayMonth    *int `gorm:"type:smallint" json:"birthdayMonth,omitempty"`
	BirthdayDay      *int `gorm:"type:smallint" json:"birthdayDay,omitempty"`
	AnniversaryMonth *int `gorm:"type:smallint" json:"anniversaryMonth,omitempty"`
	AnniversaryDay   *int `gorm:"type:smallint" json:"anniversaryDay,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (WorkforceMember) TableName() string {
	return "workforce_members"
}

// WorkforceLookupResult is the minimal, PII-conscious payload returned by the
// public email-lookup endpoint — deliberately excludes ID, Notes, and other
// internal fields since the caller only proves they know the email address,
// not that they own the record.
type WorkforceLookupResult struct {
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	Phone      string `json:"phone,omitempty"`
	Department string `json:"department"`
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

	BirthdayMonth    *int    `json:"birthdayMonth,omitempty"`
	BirthdayDay      *int    `json:"birthdayDay,omitempty"`
	Birthday         *string `json:"birthday,omitempty"` // DD/MM
	AnniversaryMonth *int    `json:"anniversaryMonth,omitempty"`
	AnniversaryDay   *int    `json:"anniversaryDay,omitempty"`
	Anniversary      *string `json:"anniversary,omitempty"` // DD/MM
}

type UpdateWorkforceRequest struct {
	FirstName  *string          `json:"firstName,omitempty"`
	LastName   *string          `json:"lastName,omitempty"`
	Email      *string          `json:"email,omitempty" binding:"omitempty,email"`
	Phone      *string          `json:"phone,omitempty"`
	Department *string          `json:"department,omitempty"`
	Status     *WorkforceStatus `json:"status,omitempty" binding:"omitempty,oneof=pending new serving not_serving"`
	Notes      *string          `json:"notes,omitempty"`

	BirthdayMonth    *int    `json:"birthdayMonth,omitempty"`
	BirthdayDay      *int    `json:"birthdayDay,omitempty"`
	Birthday         *string `json:"birthday,omitempty"` // DD/MM
	AnniversaryMonth *int    `json:"anniversaryMonth,omitempty"`
	AnniversaryDay   *int    `json:"anniversaryDay,omitempty"`
	Anniversary      *string `json:"anniversary,omitempty"` // DD/MM
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
