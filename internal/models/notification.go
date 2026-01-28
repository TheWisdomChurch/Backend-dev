// internal/models/notification.go
package models

import (
	"time"

	"gorm.io/gorm"
)

type NotificationType string

type NotificationDeliveryStatus string

const (
	NotificationTypeUpdate NotificationType = "update"
	NotificationTypeEvent  NotificationType = "event"
)

const (
	DeliveryStatusPending NotificationDeliveryStatus = "pending"
	DeliveryStatusSent    NotificationDeliveryStatus = "sent"
	DeliveryStatusFailed  NotificationDeliveryStatus = "failed"
)

type Notification struct {
	ID      string           `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	Type    NotificationType `gorm:"size:20;not null" json:"type"`
	Subject string           `gorm:"size:255;not null" json:"subject"`
	Title   string           `gorm:"size:255;not null" json:"title"`
	Message string           `gorm:"type:text;not null" json:"message"`
	EventID *string          `gorm:"type:uuid;index" json:"eventId,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (Notification) TableName() string {
	return "notifications"
}

type NotificationDelivery struct {
	ID             string                     `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	NotificationID string                     `gorm:"type:uuid;not null;index" json:"notificationId"`
	SubscriberID   string                     `gorm:"type:uuid;not null;index" json:"subscriberId"`
	Status         NotificationDeliveryStatus `gorm:"size:20;not null;default:'pending'" json:"status"`
	ErrorMessage   *string                    `gorm:"type:text" json:"errorMessage,omitempty"`
	SentAt         *time.Time                 `json:"sentAt,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (NotificationDelivery) TableName() string {
	return "notification_deliveries"
}

type SendNotificationRequest struct {
	Type    string  `json:"type" binding:"required"`
	Subject string  `json:"subject" binding:"required"`
	Title   string  `json:"title" binding:"required"`
	Message string  `json:"message" binding:"required"`
	EventID *string `json:"eventId,omitempty"`
}

type SendNotificationResult struct {
	Notification *Notification `json:"notification"`
	Total        int           `json:"total"`
	Sent         int           `json:"sent"`
	Failed       int           `json:"failed"`
}
