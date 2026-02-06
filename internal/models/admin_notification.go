package models

import "time"

type AdminNotification struct {
	ID string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`

	UserID string `gorm:"type:uuid;index;not null" json:"userId"`

	Type    string `gorm:"type:varchar(40);not null" json:"type"`
	Title   string `gorm:"type:varchar(255);not null" json:"title"`
	Message string `gorm:"type:text;not null" json:"message"`

	TicketCode *string `gorm:"type:varchar(50)" json:"ticketCode,omitempty"`
	EntityType *string `gorm:"type:varchar(40)" json:"entityType,omitempty"`
	EntityID   *string `gorm:"type:uuid" json:"entityId,omitempty"`

	IsRead bool       `gorm:"not null;default:false" json:"isRead"`
	ReadAt *time.Time `json:"readAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

func (AdminNotification) TableName() string {
	return "admin_notifications"
}
