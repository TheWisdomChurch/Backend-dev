package service

import (
	"testing"

	"wisdomHouse-backend/internal/models"
)

func TestValueAsStringMatchesGeneratedFormKeys(t *testing.T) {
	values := map[string]any{
		"first-name":     "Ada",
		"last-name":      "Lovelace",
		"contact-number": "+2348012345678",
	}

	if got := valueAsString(values, "firstName"); got != "Ada" {
		t.Fatalf("firstName lookup = %q, want Ada", got)
	}
	if got := valueAsString(values, "last_name"); got != "Lovelace" {
		t.Fatalf("last_name lookup = %q, want Lovelace", got)
	}
	if got := valueAsString(values, "contactNumber"); got != "+2348012345678" {
		t.Fatalf("contactNumber lookup = %q, want phone", got)
	}
}

func TestBuildLeadershipRequestFromFormBuilderValues(t *testing.T) {
	values := map[string]any{
		"first-name":      "Ada",
		"last-name":       "Lovelace",
		"email-address":   "ada@example.com",
		"phone-number":    "+2348012345678",
		"leadership-role": "Senior Pastor",
		"profile-image":   "https://cdn.example.com/ada.webp",
		"about":           "Leads the outreach team.",
	}

	req, err := buildLeadershipRequest(values)
	if err != nil {
		t.Fatalf("buildLeadershipRequest returned error: %v", err)
	}

	if req.FirstName != "Ada" || req.LastName != "Lovelace" {
		t.Fatalf("name = %q %q, want Ada Lovelace", req.FirstName, req.LastName)
	}
	if req.Email != "ada@example.com" {
		t.Fatalf("email = %q, want ada@example.com", req.Email)
	}
	if req.Phone != "+2348012345678" {
		t.Fatalf("phone = %q, want +2348012345678", req.Phone)
	}
	// Role is stored exactly as the applicant typed it — never forced to a slug.
	if req.Role != models.LeadershipRole("Senior Pastor") {
		t.Fatalf("role = %q, want %q", req.Role, "Senior Pastor")
	}
	if req.ImageURL == nil || *req.ImageURL != "https://cdn.example.com/ada.webp" {
		t.Fatalf("imageURL = %v, want profile image", req.ImageURL)
	}
}

func TestSubmissionFieldValuesRemovesConsentMetadataBeforeValidation(t *testing.T) {
	values := map[string]any{
		"email":            "ada@example.com",
		"_consentAccepted": true,
		"_consentVersion":  "2026-08",
	}

	got := submissionFieldValues(values)
	if got["email"] != "ada@example.com" {
		t.Fatalf("email = %v, want ada@example.com", got["email"])
	}
	if _, exists := got["_consentAccepted"]; exists {
		t.Fatal("_consentAccepted must not be validated as a configured form field")
	}
	if _, exists := got["_consentVersion"]; exists {
		t.Fatal("_consentVersion must not be validated as a configured form field")
	}

	// Sanitizing the validation input must not mutate the request because Submit
	// reads consent acceptance separately when it records server-owned metadata.
	if values["_consentAccepted"] != true || values["_consentVersion"] != "2026-08" {
		t.Fatal("submissionFieldValues mutated the request values")
	}
}

func TestSubmissionFieldValuesKeepsUnknownUserFieldsForStrictValidation(t *testing.T) {
	values := map[string]any{"unexpected": "value"}
	got := submissionFieldValues(values)

	if got["unexpected"] != "value" {
		t.Fatal("ordinary unknown fields must remain for strict validation")
	}
}

func TestBuildLeadershipRequestFromLeadershipPreset(t *testing.T) {
	values := map[string]any{
		"full_name":       "Grace Hopper",
		"email":           "grace@example.com",
		"leadership_role": "Head of Ushering",
		"photo":           "https://cdn.example.com/grace.webp",
	}

	req, err := buildLeadershipRequest(values)
	if err != nil {
		t.Fatalf("buildLeadershipRequest returned error: %v", err)
	}
	// An arbitrary role the applicant typed is preserved verbatim.
	if req.Role != models.LeadershipRole("Head of Ushering") {
		t.Fatalf("role = %q, want %q", req.Role, "Head of Ushering")
	}
	if req.ImageURL == nil || *req.ImageURL != "https://cdn.example.com/grace.webp" {
		t.Fatalf("imageURL = %v, want preset photo", req.ImageURL)
	}
}

func TestNormalizePublicFormDateValue(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		fullYear bool
		want     string
		wantErr  bool
	}{
		{"day-month keeps only DD-MM", "24-12-1990", false, "24-12", false},
		{"day-month from slashes", "07/03", false, "07-03", false},
		{"full keeps the year", "24-12-1990", true, "24-12-1990", false},
		{"full from slashes", "01/06/2018", true, "01-06-2018", false},
		{"full from ISO", "2016-09-05", true, "05-09-2016", false},
		{"full two-digit year expands", "1-1-19", true, "01-01-2019", false},
		{"full requires a year", "24-12", true, "", true},
		{"invalid month", "24-13-2000", true, "", true},
		{"garbage", "not a date", false, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePublicFormDateValue(tc.in, tc.fullYear)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalize(%q, full=%v) = %q, want %q", tc.in, tc.fullYear, got, tc.want)
			}
		})
	}
}
