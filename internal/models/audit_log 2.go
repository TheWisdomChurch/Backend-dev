package models

import "time"

// AuditLog is a durable record of a mutating admin/auth request, persisted so
// admin screens (recent activity, audit log views) have real data instead of
// only structured log lines. Written by internal/middleware/audit.go.
type AuditLog struct {
	ID         string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Scope      string    `gorm:"type:varchar(50);not null;index" json:"scope"`
	Method     string    `gorm:"type:varchar(10);not null" json:"method"`
	Path       string    `gorm:"type:varchar(500);not null" json:"path"`
	StatusCode int       `gorm:"not null" json:"statusCode"`
	LatencyMS  int64     `gorm:"not null" json:"latencyMs"`
	UserID     *string   `gorm:"type:uuid;index" json:"userId,omitempty"`
	Role       string    `gorm:"type:varchar(50)" json:"role"`
	IP         string    `gorm:"type:varchar(64)" json:"ip"`
	UserAgent  string    `gorm:"type:varchar(500)" json:"userAgent"`
	RequestID  string    `gorm:"type:varchar(100);index" json:"requestId"`
	CreatedAt  time.Time `gorm:"not null;index" json:"createdAt"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
