package models

import "time"

type ApprovalRequestType string
type ApprovalRequestStatus string

const (
	ApprovalTypeTestimonial           ApprovalRequestType = "testimonial"
	ApprovalTypeTestimonialDelete     ApprovalRequestType = "testimonial_delete"
	ApprovalTypeEvent                 ApprovalRequestType = "event"
	ApprovalTypeEventDelete           ApprovalRequestType = "event_delete"
	ApprovalTypeAdminUser             ApprovalRequestType = "admin_user"
	ApprovalTypeLeadershipDelete      ApprovalRequestType = "leadership_delete"
	ApprovalTypeWorkforceDelete       ApprovalRequestType = "workforce_delete"
	ApprovalTypeWorkforceRegistration ApprovalRequestType = "workforce_registration"
)

const (
	ApprovalStatusPending  ApprovalRequestStatus = "pending"
	ApprovalStatusApproved ApprovalRequestStatus = "approved"
	ApprovalStatusRejected ApprovalRequestStatus = "rejected"
	ApprovalStatusDeleted  ApprovalRequestStatus = "deleted"
)

type ApprovalRequest struct {
	ID         string                `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	TicketCode string                `gorm:"type:varchar(50);uniqueIndex;not null" json:"ticketCode"`
	Type       ApprovalRequestType   `gorm:"type:varchar(30);not null" json:"type"`
	Status     ApprovalRequestStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`

	EntityID    *string `gorm:"type:uuid" json:"entityId,omitempty"`
	EntityLabel *string `gorm:"type:varchar(255)" json:"entityLabel,omitempty"`
	// Reason is the requester's stated justification — required for
	// delete-approval requests so the super-admin has real context, not just
	// an entity label, before approving an irreversible action.
	Reason *string `gorm:"type:text" json:"reason,omitempty"`

	RequestedByID    *string `gorm:"type:uuid" json:"requestedById,omitempty"`
	RequestedByName  *string `gorm:"type:varchar(120)" json:"requestedByName,omitempty"`
	RequestedByEmail *string `gorm:"type:varchar(255)" json:"requestedByEmail,omitempty"`

	ApprovedByID    *string    `gorm:"type:uuid" json:"approvedById,omitempty"`
	ApprovedByName  *string    `gorm:"type:varchar(120)" json:"approvedByName,omitempty"`
	ApprovedByEmail *string    `gorm:"type:varchar(255)" json:"approvedByEmail,omitempty"`
	ApprovedAt      *time.Time `json:"approvedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (ApprovalRequest) TableName() string {
	return "approval_requests"
}
