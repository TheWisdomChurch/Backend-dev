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
	ID         string          `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName  string          `gorm:"size:100;not null" json:"firstName"`
	LastName   string          `gorm:"size:100;not null" json:"lastName"`
	Email      string          `gorm:"size:255;index" json:"email"`
	Phone      string          `gorm:"size:50" json:"phone"`
	Department string          `gorm:"size:120;index;not null" json:"department"`
	Status     WorkforceStatus `gorm:"size:20;not null;default:'pending'" json:"status"`
	Notes      *string         `gorm:"type:text" json:"notes,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (WorkforceMember) TableName() string {
	return "workforce_members"
}

type CreateWorkforceRequest struct {
	FirstName  string          `json:"firstName" binding:"required"`
	LastName   string          `json:"lastName" binding:"required"`
	Email      string          `json:"email" binding:"omitempty,email"`
	Phone      string          `json:"phone"`
	Department string          `json:"department" binding:"required"`
	Status     WorkforceStatus `json:"status" binding:"omitempty,oneof=pending new serving not_serving"`
	Notes      *string         `json:"notes,omitempty"`
}

type UpdateWorkforceRequest struct {
	FirstName  *string          `json:"firstName,omitempty"`
	LastName   *string          `json:"lastName,omitempty"`
	Email      *string          `json:"email,omitempty"`
	Phone      *string          `json:"phone,omitempty"`
	Department *string          `json:"department,omitempty"`
	Status     *WorkforceStatus `json:"status,omitempty" binding:"omitempty,oneof=pending new serving not_serving"`
	Notes      *string          `json:"notes,omitempty"`
}

type WorkforceStatsResponse struct {
	Total           int64             `json:"total"`
	ByStatus        map[string]int64  `json:"byStatus"`
	ByDepartment    map[string]int64  `json:"byDepartment"`
	ByDeptAndStatus []WorkforceBucket `json:"byDeptAndStatus"`
}

type WorkforceBucket struct {
	Department string `json:"department"`
	Status     string `json:"status"`
	Count      int64  `json:"count"`
}
