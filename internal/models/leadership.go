package models

import "time"

type LeadershipStatus string

const (
	LeadershipStatusPending  LeadershipStatus = "pending"
	LeadershipStatusApproved LeadershipStatus = "approved"
)

type LeadershipRole string

const (
	LeadershipRoleAssociatePastor LeadershipRole = "associate_pastor"
	LeadershipRoleDeacon          LeadershipRole = "deacon"
	LeadershipRoleDeaconess       LeadershipRole = "deaconess"
	LeadershipRoleReverend        LeadershipRole = "reverend"
)

type LeadershipMember struct {
	ID        string           `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName string           `gorm:"size:100;not null" json:"firstName"`
	LastName  string           `gorm:"size:100;not null" json:"lastName"`
	Email     string           `gorm:"size:255;index" json:"email"`
	Phone     string           `gorm:"size:50" json:"phone"`
	Role      LeadershipRole   `gorm:"size:30;not null" json:"role"`
	Status    LeadershipStatus `gorm:"size:20;not null;default:'pending'" json:"status"`
	Bio       *string          `gorm:"type:text" json:"bio,omitempty"`
	ImageURL  *string          `gorm:"type:text" json:"imageUrl,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (LeadershipMember) TableName() string {
	return "leadership_members"
}

type CreateLeadershipRequest struct {
	FirstName string           `json:"firstName" binding:"required"`
	LastName  string           `json:"lastName" binding:"required"`
	Email     string           `json:"email" binding:"omitempty,email"`
	Phone     string           `json:"phone"`
	Role      LeadershipRole   `json:"role" binding:"required,oneof=associate_pastor deacon deaconess reverend"`
	Status    LeadershipStatus `json:"status" binding:"omitempty,oneof=pending approved"`
	Bio       *string          `json:"bio,omitempty"`
	ImageURL  *string          `json:"imageUrl,omitempty"`
}

type UpdateLeadershipRequest struct {
	FirstName *string           `json:"firstName,omitempty"`
	LastName  *string           `json:"lastName,omitempty"`
	Email     *string           `json:"email,omitempty"`
	Phone     *string           `json:"phone,omitempty"`
	Role      *LeadershipRole   `json:"role,omitempty" binding:"omitempty,oneof=associate_pastor deacon deaconess reverend"`
	Status    *LeadershipStatus `json:"status,omitempty" binding:"omitempty,oneof=pending approved"`
	Bio       *string           `json:"bio,omitempty"`
	ImageURL  *string           `json:"imageUrl,omitempty"`
}
