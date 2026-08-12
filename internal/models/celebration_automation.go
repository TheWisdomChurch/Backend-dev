package models

import (
	"time"

	"gorm.io/datatypes"
)

type CelebrationAutomationConfig struct {
	ID                     string     `gorm:"primaryKey;type:varchar(40)" json:"id"`
	Enabled                bool       `gorm:"not null;default:false" json:"enabled"`
	BirthdayEnabled        bool       `gorm:"not null;default:true" json:"birthdayEnabled"`
	AnniversaryEnabled     bool       `gorm:"not null;default:true" json:"anniversaryEnabled"`
	Timezone               string     `gorm:"type:varchar(80);not null" json:"timezone"`
	SendTime               string     `gorm:"type:char(5);not null" json:"sendTime"`
	Feb29Policy            string     `gorm:"type:varchar(12);not null;default:'feb28'" json:"feb29Policy"`
	MaxAttempts            int        `gorm:"not null;default:3" json:"maxAttempts"`
	RetryMinutes           int        `gorm:"not null;default:15" json:"retryMinutes"`
	BirthdaySubject        string     `gorm:"type:varchar(180);not null" json:"birthdaySubject"`
	AnniversarySubject     string     `gorm:"type:varchar(180);not null" json:"anniversarySubject"`
	BirthdayTemplateKey    string     `gorm:"type:varchar(120);not null;default:'birthday'" json:"birthdayTemplateKey"`
	AnniversaryTemplateKey string     `gorm:"type:varchar(120);not null;default:'anniversary'" json:"anniversaryTemplateKey"`
	UpdatedByUserID        *string    `gorm:"type:uuid" json:"updatedByUserId,omitempty"`
	UpdatedByEmail         *string    `gorm:"type:varchar(255)" json:"updatedByEmail,omitempty"`
	LastWorkerHeartbeat    *time.Time `json:"lastWorkerHeartbeat,omitempty"`
	LastWorkerID           *string    `gorm:"type:varchar(120)" json:"lastWorkerId,omitempty"`
	CreatedAt              time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt              time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (CelebrationAutomationConfig) TableName() string { return "celebration_automation_config" }

type CelebrationAutomationRun struct {
	ID             string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	RunDate        string         `gorm:"type:date;not null;uniqueIndex" json:"runDate"`
	Timezone       string         `gorm:"type:varchar(80);not null" json:"timezone"`
	Status         string         `gorm:"type:varchar(20);not null;index" json:"status"`
	Attempt        int            `gorm:"not null;default:1" json:"attempt"`
	Targeted       int            `gorm:"not null;default:0" json:"targeted"`
	Sent           int            `gorm:"not null;default:0" json:"sent"`
	Suppressed     int            `gorm:"not null;default:0" json:"suppressed"`
	Skipped        int            `gorm:"not null;default:0" json:"skipped"`
	Failed         int            `gorm:"not null;default:0" json:"failed"`
	LastError      *string        `gorm:"type:text" json:"lastError,omitempty"`
	NextAttemptAt  *time.Time     `gorm:"index" json:"nextAttemptAt,omitempty"`
	ClaimedAt      *time.Time     `gorm:"index" json:"-"`
	ClaimedBy      *string        `gorm:"type:varchar(120)" json:"-"`
	Trigger        string         `gorm:"type:varchar(30);not null" json:"trigger"`
	ConfigSnapshot datatypes.JSON `gorm:"type:jsonb;not null" json:"configSnapshot"`
	StartedAt      *time.Time     `json:"startedAt,omitempty"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (CelebrationAutomationRun) TableName() string { return "celebration_automation_runs" }

type CelebrationDelivery struct {
	ID             string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	RunID          string         `gorm:"type:uuid;not null;index;uniqueIndex:ux_celebration_delivery" json:"runId"`
	Kind           string         `gorm:"type:varchar(20);not null;uniqueIndex:ux_celebration_delivery" json:"kind"`
	EmailHash      string         `gorm:"type:char(64);not null;uniqueIndex:ux_celebration_delivery" json:"-"`
	RecipientEmail string         `gorm:"type:varchar(255);not null" json:"recipientEmail"`
	RecipientName  string         `gorm:"type:varchar(220);not null" json:"recipientName"`
	Sources        datatypes.JSON `gorm:"type:jsonb;not null" json:"sources"`
	Status         string         `gorm:"type:varchar(20);not null;index" json:"status"`
	Attempt        int            `gorm:"not null;default:0" json:"attempt"`
	Error          *string        `gorm:"type:text" json:"error,omitempty"`
	SentAt         *time.Time     `json:"sentAt,omitempty"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (CelebrationDelivery) TableName() string { return "celebration_deliveries" }

type UpdateCelebrationAutomationConfigRequest struct {
	Enabled                bool   `json:"enabled"`
	BirthdayEnabled        bool   `json:"birthdayEnabled"`
	AnniversaryEnabled     bool   `json:"anniversaryEnabled"`
	Timezone               string `json:"timezone" binding:"required"`
	SendTime               string `json:"sendTime" binding:"required"`
	Feb29Policy            string `json:"feb29Policy" binding:"required"`
	MaxAttempts            int    `json:"maxAttempts"`
	RetryMinutes           int    `json:"retryMinutes"`
	BirthdaySubject        string `json:"birthdaySubject" binding:"required"`
	AnniversarySubject     string `json:"anniversarySubject" binding:"required"`
	BirthdayTemplateKey    string `json:"birthdayTemplateKey" binding:"required"`
	AnniversaryTemplateKey string `json:"anniversaryTemplateKey" binding:"required"`
}

type CelebrationAutomationStatus struct {
	Config        CelebrationAutomationConfig `json:"config"`
	TodayRun      *CelebrationAutomationRun   `json:"todayRun,omitempty"`
	NextRunAt     *time.Time                  `json:"nextRunAt,omitempty"`
	WorkerHealthy bool                        `json:"workerHealthy"`
}
