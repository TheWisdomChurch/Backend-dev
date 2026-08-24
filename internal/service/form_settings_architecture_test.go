package service

import (
	"encoding/json"
	"testing"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
)

func TestFormPresentationSettingsHaveSingleCanonicalRepresentation(t *testing.T) {
	legacyTitle := "Nested title"
	canonicalTitle := "Root title"
	legacySections := []models.FormContentSectionDTO{{Title: "About"}}
	settings := &models.FormSettingsDTO{
		IntroTitle: &canonicalTitle,
		Sections:   &legacySections,
		Design:     &models.FormDesignSettingsDTO{IntroTitle: &legacyTitle},
	}

	encoded, err := encodeSettings(settings)
	if err != nil {
		t.Fatalf("encode settings: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode encoded settings: %v", err)
	}
	if payload["introTitle"] != canonicalTitle {
		t.Fatalf("introTitle = %v, want %q", payload["introTitle"], canonicalTitle)
	}
	if _, exists := payload["sections"]; exists {
		t.Fatal("settings must not emit duplicate sections")
	}
	design, _ := payload["design"].(map[string]any)
	if _, exists := design["introTitle"]; exists {
		t.Fatal("settings.design must not duplicate root introTitle")
	}
	if _, exists := payload["contentSections"]; !exists {
		t.Fatal("sections must migrate to canonical contentSections")
	}
}

func TestDecodeSettingsMigratesNestedPresentationValues(t *testing.T) {
	decoded, err := decodeSettings(datatypes.JSON([]byte(`{"design":{"introTitle":"Legacy title","footerBg":"#ffffff"},"sections":[{"title":"About"}]}`)))
	if err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if decoded.IntroTitle == nil || *decoded.IntroTitle != "Legacy title" {
		t.Fatalf("introTitle was not migrated: %v", decoded.IntroTitle)
	}
	if decoded.FooterBg == nil || *decoded.FooterBg != "#ffffff" {
		t.Fatalf("footerBg was not migrated: %v", decoded.FooterBg)
	}
	if decoded.Sections != nil || decoded.Design.IntroTitle != nil || decoded.Design.FooterBg != nil {
		t.Fatal("legacy duplicate values must be cleared after migration")
	}
	if decoded.ContentSections == nil || len(*decoded.ContentSections) != 1 {
		t.Fatal("sections were not migrated to contentSections")
	}
}

func TestDecodeSettingsUpgradesLegacyFormArchitecture(t *testing.T) {
	settings, err := decodeSettings(datatypes.JSON([]byte(`{"layoutMode":"stack"}`)))
	if err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if settings.RendererVersion == nil || *settings.RendererVersion != 2 {
		t.Fatalf("expected renderer version 2, got %#v", settings.RendererVersion)
	}
	if settings.Consent == nil || settings.Consent.Required == nil || !*settings.Consent.Required {
		t.Fatal("expected required consent defaults")
	}
	if settings.Consent.Introduction == nil || len(*settings.Consent.Introduction) < 200 {
		t.Fatal("expected substantive consent introduction")
	}
	if settings.Consent.Purposes == nil || len(*settings.Consent.Purposes) < 4 {
		t.Fatal("expected structured consent purposes")
	}
}

func TestEncodeSettingsDoesNotAllowConsentToBeDisabled(t *testing.T) {
	settings, err := decodeSettings(nil)
	if err != nil {
		t.Fatal(err)
	}
	no := false
	settings.Consent.Enabled = &no
	settings.Consent.Required = &no
	if _, err := encodeSettings(settings); err != nil {
		t.Fatalf("encode settings: %v", err)
	}
	if !*settings.Consent.Enabled || !*settings.Consent.Required {
		t.Fatal("all forms must retain required consent")
	}
}
