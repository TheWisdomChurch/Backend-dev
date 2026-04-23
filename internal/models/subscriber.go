// internal/models/subscriber.go
package models

import (
	"time"

	"gorm.io/gorm"
)

type SubscriberStatus string

const (
	SubscriberStatusActive       SubscriberStatus = "active"
	SubscriberStatusUnsubscribed SubscriberStatus = "unsubscribed"
)

type Subscriber struct {
	ID             string           `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	Email          string           `gorm:"size:255;uniqueIndex;not null" json:"email"`
	Name           *string          `gorm:"size:120" json:"name,omitempty"`
	Source         *string          `gorm:"size:120" json:"source,omitempty"`
	Status         SubscriberStatus `gorm:"size:20;not null;default:'active'" json:"status"`
	UnsubscribedAt *time.Time       `json:"unsubscribedAt,omitempty"`
	LastNotifiedAt *time.Time       `json:"lastNotifiedAt,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Subscriber) TableName() string {
	return "subscribers"
}

type SubscribeRequest struct {
	Email  string  `json:"email" binding:"required,email"`
	Name   *string `json:"name,omitempty"`
	Source *string `json:"source,omitempty"`
}

type UnsubscribeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type SubscriberSummary struct {
	Total               int64      `json:"total"`
	Active              int64      `json:"active"`
	Unsubscribed        int64      `json:"unsubscribed"`
	RecentlyAdded30d    int64      `json:"recentlyAdded30d"`
	RecentlyOptedOut30d int64      `json:"recentlyOptedOut30d"`
	LastNotifiedAt      *time.Time `json:"lastNotifiedAt,omitempty"`
}
