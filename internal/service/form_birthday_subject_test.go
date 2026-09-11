package service

import (
	"testing"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
)

func TestExtractFormBirthdaySubject(t *testing.T) {
	name := "Ada Lovelace"
	emailAddr := "ADA@Example.com "

	dobField := models.FormField{
		Key:        "child_date_of_birth",
		Label:      "Child's date of birth",
		Type:       models.FieldDate,
		Validation: datatypes.JSON([]byte(`{"dateMode":"full"}`)),
	}
	dayMonthField := models.FormField{
		Key:        "preferred_visit_date",
		Label:      "Preferred visit date",
		Type:       models.FieldDate,
		Validation: nil,
	}
	form := &models.Form{ID: "form-1", Fields: []models.FormField{dayMonthField, dobField}}

	t.Run("finds the full-date field and normalises the email", func(t *testing.T) {
		values := map[string]any{
			"preferred_visit_date": "12-03",
			"child_date_of_birth":  "05-09-2016",
		}
		got := extractFormBirthdaySubject(form, values, &name, &emailAddr, "sub-1")
		if got == nil {
			t.Fatal("expected a subject, got nil")
		}
		if got.FieldKey != "child_date_of_birth" {
			t.Errorf("field key = %q, want child_date_of_birth", got.FieldKey)
		}
		if got.BirthdayDay != 5 || got.BirthdayMonth != 9 {
			t.Errorf("day/month = %d/%d, want 5/9", got.BirthdayDay, got.BirthdayMonth)
		}
		if got.Email != "ada@example.com" {
			t.Errorf("email = %q, want normalised lowercase", got.Email)
		}
		if got.FirstName != "Ada" || got.LastName != "Lovelace" {
			t.Errorf("name = %q %q", got.FirstName, got.LastName)
		}
		if got.FormID != "form-1" || got.SubmissionID != "sub-1" {
			t.Errorf("form/submission id wrong: %+v", got)
		}
	})

	t.Run("no email means no subject", func(t *testing.T) {
		values := map[string]any{"child_date_of_birth": "05-09-2016"}
		if got := extractFormBirthdaySubject(form, values, &name, nil, "sub-2"); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
		blank := "  "
		if got := extractFormBirthdaySubject(form, values, &name, &blank, "sub-2"); got != nil {
			t.Fatalf("expected nil for blank email, got %+v", got)
		}
	})

	t.Run("day-month value on the DOB field key is rejected (no year)", func(t *testing.T) {
		values := map[string]any{"child_date_of_birth": "05-09"}
		if got := extractFormBirthdaySubject(form, values, &name, &emailAddr, "sub-3"); got != nil {
			t.Fatalf("expected nil (missing year), got %+v", got)
		}
	})

	t.Run("no date-of-birth field on the form", func(t *testing.T) {
		plain := &models.Form{ID: "form-2", Fields: []models.FormField{dayMonthField}}
		values := map[string]any{"preferred_visit_date": "12-03"}
		if got := extractFormBirthdaySubject(plain, values, &name, &emailAddr, "sub-4"); got != nil {
			t.Fatalf("expected nil, got %+v", got)
		}
	})
}
