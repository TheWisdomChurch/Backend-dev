package models

import "time"

type LeadershipStatus string

const (
	LeadershipStatusPending                    LeadershipStatus = "pending"
	LeadershipStatusAwaitingSuperAdminApproval LeadershipStatus = "awaiting_super_admin_approval"
	LeadershipStatusApproved                   LeadershipStatus = "approved"
	LeadershipStatusDeclined                   LeadershipStatus = "declined"
)

type LeadershipRole string

const (
	LeadershipRoleSeniorPastor    LeadershipRole = "senior_pastor"
	LeadershipRoleAssociatePastor LeadershipRole = "associate_pastor"
	LeadershipRoleDeacon          LeadershipRole = "deacon"
	LeadershipRoleDeaconess       LeadershipRole = "deaconess"
	LeadershipRoleReverend        LeadershipRole = "reverend"
)

type LeadershipMember struct {
	ID        string           `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FirstName string           `gorm:"size:100;not null" json:"firstName"`
	LastName  string           `gorm:"size:100;not null" json:"lastName"`
	Email     *string          `gorm:"size:255;index:idx_leadership_members_email" json:"email,omitempty"`
	Phone     *string          `gorm:"size:50" json:"phone,omitempty"`
	Role      LeadershipRole   `gorm:"size:30;not null;index:idx_leadership_role_status" json:"role"`
	Status    LeadershipStatus `gorm:"size:40;not null;default:'pending';index:idx_leadership_role_status;index:idx_leadership_status" json:"status"`
	Bio       *string          `gorm:"type:text" json:"bio,omitempty"`
	ImageURL  *string          `gorm:"type:text" json:"imageUrl,omitempty"`

	// Date components are stored as month/day so yearly greeting automation works without storing full birth years.
	BirthdayMonth    *int `gorm:"type:smallint" json:"birthdayMonth,omitempty"`
	BirthdayDay      *int `gorm:"type:smallint" json:"birthdayDay,omitempty"`
	AnniversaryMonth *int `gorm:"type:smallint" json:"anniversaryMonth,omitempty"`
	AnniversaryDay   *int `gorm:"type:smallint" json:"anniversaryDay,omitempty"`

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
	Role      LeadershipRole   `json:"role" binding:"required,oneof=senior_pastor associate_pastor deacon deaconess reverend"`
	Status    LeadershipStatus `json:"status" binding:"omitempty,oneof=pending awaiting_super_admin_approval approved declined"`
	Bio       *string          `json:"bio,omitempty"`
	ImageURL  *string          `json:"imageUrl,omitempty"`

	Birthday    *string `json:"birthday,omitempty"`    // DD/MM
	Anniversary *string `json:"anniversary,omitempty"` // DD/MM or DD/MM/YYYY, parsed by service layer

	BirthdayMonth    *int `json:"birthdayMonth,omitempty"`
	BirthdayDay      *int `json:"birthdayDay,omitempty"`
	AnniversaryMonth *int `json:"anniversaryMonth,omitempty"`
	AnniversaryDay   *int `json:"anniversaryDay,omitempty"`
}

type UpdateLeadershipRequest struct {
	FirstName *string           `json:"firstName,omitempty"`
	LastName  *string           `json:"lastName,omitempty"`
	Email     *string           `json:"email,omitempty" binding:"omitempty,email"`
	Phone     *string           `json:"phone,omitempty"`
	Role      *LeadershipRole   `json:"role,omitempty" binding:"omitempty,oneof=senior_pastor associate_pastor deacon deaconess reverend"`
	Status    *LeadershipStatus `json:"status,omitempty" binding:"omitempty,oneof=pending awaiting_super_admin_approval approved declined"`
	Bio       *string           `json:"bio,omitempty"`
	ImageURL  *string           `json:"imageUrl,omitempty"`

	BirthdayMonth    *int    `json:"birthdayMonth,omitempty"`
	BirthdayDay      *int    `json:"birthdayDay,omitempty"`
	Birthday         *string `json:"birthday,omitempty"` // DD/MM
	AnniversaryMonth *int    `json:"anniversaryMonth,omitempty"`
	AnniversaryDay   *int    `json:"anniversaryDay,omitempty"`
	Anniversary      *string `json:"anniversary,omitempty"` // DD/MM or DD/MM/YYYY
}
