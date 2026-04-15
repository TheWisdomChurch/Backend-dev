package models

import (
	"time"

	"gorm.io/datatypes"
)

type AnalyticsBatch struct {
	ID         string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BatchID    string `gorm:"size:120;index;not null" json:"batchId"`
	SessionID  string `gorm:"size:120;index;not null" json:"sessionId"`
	UserID     *string `gorm:"size:120;index" json:"userId,omitempty"`
	EventCount int    `gorm:"not null;default:0" json:"eventCount"`
	Payload    datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func (AnalyticsBatch) TableName() string {
	return "analytics_batches"
}

