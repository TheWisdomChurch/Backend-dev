// internal/models/form_submission.go
package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type FormSubmission struct {
	ID string `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`

	FormID string `gorm:"type:uuid;not null;index" json:"formId"`

	// Common denormalized fields for quick analytics
	Name             *string `gorm:"size:255" json:"name,omitempty"`
	Email            *string `gorm:"size:255" json:"email,omitempty"`
	ContactNumber    *string `gorm:"size:100" json:"contactNumber,omitempty"`
	ContactAddress   *string `gorm:"size:500" json:"contactAddress,omitempty"`
	RegistrationCode *string `gorm:"size:40" json:"registrationCode,omitempty"`

	// Values: {"fullName":"...", "agree":true, ...}
	Values datatypes.JSON `gorm:"type:jsonb;not null" json:"values"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (FormSubmission) TableName() string {
	return "form_submissions"
}

// FormSubmissionWithForm is used for admin stats/list endpoints to return form title alongside submission.
type FormSubmissionWithForm struct {
	ID               string         `json:"id"`
	FormID           string         `json:"formId"`
	FormTitle        string         `json:"formTitle"`
	Name             *string        `json:"name,omitempty"`
	Email            *string        `json:"email,omitempty"`
	ContactNumber    *string        `json:"contactNumber,omitempty"`
	ContactAddress   *string        `json:"contactAddress,omitempty"`
	RegistrationCode *string        `json:"registrationCode,omitempty"`
	Values           datatypes.JSON `json:"values"`
	CreatedAt        time.Time      `json:"createdAt"`
}

type FormSubmissionCount struct {
	FormID    string `json:"formId"`
	FormTitle string `json:"formTitle"`
	Count     int64  `json:"count"`
}

type FormSubmissionDailyCount struct {
	Day   time.Time `json:"day"`
	Count int64     `json:"count"`
}

type FormStatsResponse struct {
	TotalSubmissions int64                    `json:"totalSubmissions"`
	PerForm          []FormSubmissionCount    `json:"perForm"`
	Recent           []FormSubmissionWithForm `json:"recent"`
}
