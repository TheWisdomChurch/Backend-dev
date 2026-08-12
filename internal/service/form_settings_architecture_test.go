package service

import (
	"testing"

	"gorm.io/datatypes"
)

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
