package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AnalyticsBatch struct {
	ID         string         `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BatchID    string         `gorm:"size:120;uniqueIndex;not null" json:"batchId"`
	SessionID  string         `gorm:"size:120;index;not null" json:"sessionId"`
	UserID     *string        `gorm:"size:120;index" json:"userId,omitempty"`
	EventCount int            `gorm:"not null;default:0" json:"eventCount"`
	Payload    datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
	CreatedAt  time.Time      `json:"createdAt"`
	ExpiresAt  time.Time      `gorm:"not null;index" json:"expiresAt"`
}

// AnalyticsEvent is the normalized, queryable representation of a validated
// client event. Payload remains available on AnalyticsBatch for short-term
// diagnostics, while dashboards aggregate this bounded schema.
type AnalyticsEvent struct {
	ID            string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BatchID       string    `gorm:"size:120;not null;index" json:"batchId"`
	SessionID     string    `gorm:"size:120;not null;index" json:"sessionId"`
	UserID        *string   `gorm:"size:120;index" json:"userId,omitempty"`
	ClientEventID *string   `gorm:"size:120" json:"clientEventId,omitempty"`
	Category      string    `gorm:"size:80;not null;index" json:"category"`
	Action        string    `gorm:"size:80;not null;index" json:"action"`
	OccurredAt    time.Time `gorm:"not null;index" json:"occurredAt"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (AnalyticsEvent) TableName() string { return "analytics_events" }

func (AnalyticsBatch) TableName() string {
	return "analytics_batches"
}

func (b *AnalyticsBatch) BeforeCreate(_ *gorm.DB) error {
	if b.ExpiresAt.IsZero() {
		b.ExpiresAt = time.Now().UTC().Add(30 * 24 * time.Hour)
	}
	return nil
}
