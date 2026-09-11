package models

import "time"

// FormBirthdaySubject captures a birthday from ANY public form — not just the
// dedicated member/workforce/leadership intake forms — so it feeds the
// celebration automation's daily birthday run. Written once per submission
// (see form_service_target_sync.go syncFormBirthdaySubject), keyed on
// submission_id so a form with several date fields only contributes one row.
type FormBirthdaySubject struct {
	ID            string    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SubmissionID  string    `gorm:"type:uuid;not null;uniqueIndex" json:"submissionId"`
	FormID        string    `gorm:"type:uuid;not null;index" json:"formId"`
	FieldKey      string    `gorm:"size:120;not null" json:"fieldKey"`
	FirstName     string    `gorm:"size:120;not null;default:''" json:"firstName"`
	LastName      string    `gorm:"size:120;not null;default:''" json:"lastName"`
	Email         string    `gorm:"size:255;not null;index" json:"email"`
	BirthdayMonth int       `gorm:"type:smallint;not null;index:idx_form_birthday_subjects_month_day" json:"birthdayMonth"`
	BirthdayDay   int       `gorm:"type:smallint;not null;index:idx_form_birthday_subjects_month_day" json:"birthdayDay"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (FormBirthdaySubject) TableName() string { return "form_birthday_subjects" }
