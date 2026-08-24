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
	if req.Role != models.LeadershipRoleSeniorPastor {
		t.Fatalf("role = %q, want %q", req.Role, models.LeadershipRoleSeniorPastor)
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
