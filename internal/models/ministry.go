package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Ministry struct {
	ID          string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string         `gorm:"not null;size:150" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	CampusID    *string        `gorm:"type:uuid;index" json:"campus_id,omitempty"`
	LeaderID    *string        `gorm:"type:uuid;index" json:"leader_id,omitempty"`
	Category    string         `gorm:"size:100;index" json:"category,omitempty"` // worship, children, media, ushering, etc.
	IsActive    bool           `gorm:"default:true;not null" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (m *Ministry) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return nil
}

type MinistryMember struct {
	ID         string         `gorm:"type:uuid;primaryKey" json:"id"`
	MinistryID string         `gorm:"type:uuid;not null;uniqueIndex:idx_ministry_member" json:"ministry_id"`
	MemberID   string         `gorm:"type:uuid;not null;uniqueIndex:idx_ministry_member;index" json:"member_id"`
	Role       string         `gorm:"size:50;default:'member'" json:"role"`
	JoinedAt   time.Time      `gorm:"not null" json:"joined_at"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

type MinistryWorkforceRole string

const (
	MinistryRoleHead        MinistryWorkforceRole = "head"
	MinistryRoleDeputyHead  MinistryWorkforceRole = "deputy_head"
	MinistryRoleCoordinator MinistryWorkforceRole = "coordinator"
	MinistryRoleMember      MinistryWorkforceRole = "member"
)

// MinistryWorkforceMember is the normalized organization relationship. People
// remain authoritative in workforce_members; ministries only own assignment,
// role and ministry-specific title.
type MinistryWorkforceMember struct {
	ID                string                `gorm:"type:uuid;primaryKey" json:"id"`
	MinistryID        string                `gorm:"type:uuid;not null;index" json:"ministryId"`
	WorkforceMemberID string                `gorm:"type:uuid;not null;index" json:"workforceMemberId"`
	Role              MinistryWorkforceRole `gorm:"size:30;not null;default:'member'" json:"role"`
	Title             *string               `gorm:"size:120" json:"title,omitempty"`
	Source            string                `gorm:"size:30;not null;default:'manual'" json:"source"`
	JoinedAt          time.Time             `gorm:"not null" json:"joinedAt"`
	CreatedAt         time.Time             `json:"createdAt"`
	UpdatedAt         time.Time             `json:"updatedAt"`
	DeletedAt         gorm.DeletedAt        `gorm:"index" json:"-"`
	WorkforceMember   WorkforceMember       `gorm:"foreignKey:WorkforceMemberID" json:"workforceMember"`
}

func (m *MinistryWorkforceMember) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = time.Now().UTC()
	}
	return nil
}

func (MinistryWorkforceMember) TableName() string { return "ministry_workforce_members" }

type MinistryStructure struct {
	Ministry     Ministry                  `json:"ministry"`
	Heads        []MinistryWorkforceMember `json:"heads"`
	DeputyHeads  []MinistryWorkforceMember `json:"deputyHeads"`
	Coordinators []MinistryWorkforceMember `json:"coordinators"`
	Members      []MinistryWorkforceMember `json:"members"`
	Total        int                       `json:"total"`
}

type CreateMinistryRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	CampusID    *string `json:"campusId"`
	Category    *string `json:"category"`
}

type UpdateMinistryRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	CampusID    *string `json:"campusId"`
	Category    *string `json:"category"`
	IsActive    *bool   `json:"isActive"`
}

type AssignMinistryWorkforceRequest struct {
	WorkforceMemberID string                `json:"workforceMemberId" binding:"required"`
	Role              MinistryWorkforceRole `json:"role" binding:"omitempty,oneof=head deputy_head coordinator member"`
	Title             *string               `json:"title"`
}

func (m *MinistryMember) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = time.Now().UTC()
	}
	return nil
}
