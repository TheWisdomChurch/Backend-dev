package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CellGroup struct {
	ID          string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string         `gorm:"not null;size:150" json:"name"`
	CampusID    *string        `gorm:"type:uuid;index" json:"campus_id,omitempty"`
	LeaderID    *string        `gorm:"type:uuid;index" json:"leader_id,omitempty"`
	Zone        string         `gorm:"size:100" json:"zone,omitempty"`
	MaxCapacity int            `gorm:"default:0" json:"max_capacity"`
	IsActive    bool           `gorm:"default:true;not null" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (g *CellGroup) BeforeCreate(_ *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	return nil
}

type CellGroupMember struct {
	ID        string         `gorm:"type:uuid;primaryKey" json:"id"`
	GroupID   string         `gorm:"type:uuid;not null;uniqueIndex:idx_cell_group_member" json:"group_id"`
	MemberID  string         `gorm:"type:uuid;not null;uniqueIndex:idx_cell_group_member;index" json:"member_id"`
	Role      string         `gorm:"size:50;default:'member'" json:"role"` // member, leader, assistant
	JoinedAt  time.Time      `gorm:"not null" json:"joined_at"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (g *CellGroupMember) BeforeCreate(_ *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	if g.JoinedAt.IsZero() {
		g.JoinedAt = time.Now().UTC()
	}
	return nil
}

type CellGroupMeeting struct {
	ID            string         `gorm:"type:uuid;primaryKey" json:"id"`
	GroupID       string         `gorm:"type:uuid;not null;index" json:"group_id"`
	Date          time.Time      `gorm:"not null;index" json:"date"`
	AttendeeCount int            `gorm:"default:0" json:"attendee_count"`
	Notes         string         `gorm:"type:text" json:"notes,omitempty"`
	LedByID       *string        `gorm:"type:uuid" json:"led_by_id,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (g *CellGroupMeeting) BeforeCreate(_ *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	return nil
}
