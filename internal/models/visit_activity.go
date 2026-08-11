package models

import "time"

// VisitActivity is an append-only operational audit trail. VisitRequest holds
// the current projection; this table explains who changed it and when.
type VisitActivity struct {
	ID         string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	VisitID    string    `gorm:"type:uuid;not null;index" json:"visitId"`
	EventType  string    `gorm:"size:60;not null;index" json:"eventType"`
	FromStatus string    `gorm:"size:40" json:"fromStatus,omitempty"`
	ToStatus   string    `gorm:"size:40" json:"toStatus,omitempty"`
	ActorID    *string   `gorm:"type:uuid;index" json:"actorId,omitempty"`
	Notes      string    `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (VisitActivity) TableName() string { return "visit_activities" }
