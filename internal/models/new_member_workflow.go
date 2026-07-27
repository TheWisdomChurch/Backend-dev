package models

import (
	"time"

	"gorm.io/datatypes"
)

const (
	NewMemberStageNew                  = "new"
	NewMemberStageContactAttempted     = "contact_attempted"
	NewMemberStageContacted            = "contacted"
	NewMemberStageOrientationScheduled = "orientation_scheduled"
	NewMemberStageOrientationCompleted = "orientation_completed"
	NewMemberStageIntegrated           = "integrated"
	NewMemberStageClosed               = "closed"
)

type NewMemberWorkflow struct {
	ID                string     `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SubmissionID      string     `gorm:"type:uuid;not null;uniqueIndex" json:"submissionId"`
	Stage             string     `gorm:"size:40;not null" json:"stage"`
	AssignedOwnerID   *string    `gorm:"type:uuid;index" json:"assignedOwnerId,omitempty"`
	AssignedOwnerName *string    `gorm:"-" json:"assignedOwnerName,omitempty"`
	NextActionAt      *time.Time `json:"nextActionAt,omitempty"`
	EscalationStatus  string     `gorm:"size:30;not null" json:"escalationStatus"`
	EscalatedAt       *time.Time `json:"escalatedAt,omitempty"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	LastContactedAt   *time.Time `json:"lastContactedAt,omitempty"`
	LastReminderAt    *time.Time `json:"lastReminderAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

func (NewMemberWorkflow) TableName() string { return "new_member_workflows" }

type NewMemberContact struct {
	ID          string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	WorkflowID  string    `gorm:"type:uuid;not null;index" json:"workflowId"`
	Channel     string    `gorm:"size:30;not null" json:"channel"`
	Outcome     string    `gorm:"size:50;not null" json:"outcome"`
	Notes       *string   `json:"notes,omitempty"`
	ContactedAt time.Time `json:"contactedAt"`
	CreatedByID string    `gorm:"type:uuid;not null" json:"createdById"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (NewMemberContact) TableName() string { return "new_member_contacts" }

type NewMemberWorkflowHistory struct {
	ID         string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	WorkflowID string         `gorm:"type:uuid;not null;index" json:"workflowId"`
	EventType  string         `gorm:"size:50;not null" json:"eventType"`
	FromStage  *string        `json:"fromStage,omitempty"`
	ToStage    *string        `json:"toStage,omitempty"`
	ActorID    *string        `gorm:"type:uuid" json:"actorId,omitempty"`
	Details    datatypes.JSON `gorm:"type:jsonb;not null" json:"details"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func (NewMemberWorkflowHistory) TableName() string { return "new_member_workflow_history" }

type UpdateNewMemberWorkflowRequest struct {
	Stage           *string    `json:"stage"`
	AssignedOwnerID *string    `json:"assignedOwnerId"`
	NextActionAt    *time.Time `json:"nextActionAt"`
}

type CreateNewMemberContactRequest struct {
	Channel      string     `json:"channel" binding:"required"`
	Outcome      string     `json:"outcome" binding:"required"`
	Notes        *string    `json:"notes"`
	ContactedAt  *time.Time `json:"contactedAt"`
	NextActionAt *time.Time `json:"nextActionAt"`
}
