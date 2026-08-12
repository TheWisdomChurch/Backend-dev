package service

import (
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
)

func validCelebrationConfig() *models.CelebrationAutomationConfig {
	return &models.CelebrationAutomationConfig{Timezone: "Africa/Lagos", SendTime: "09:00", Feb29Policy: "feb28", MaxAttempts: 3, RetryMinutes: 15, BirthdaySubject: "Happy Birthday", AnniversarySubject: "Happy Anniversary", BirthdayTemplateKey: "birthday", AnniversaryTemplateKey: "anniversary"}
}

func TestCelebrationConfigValidation(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if err := configValid(validCelebrationConfig()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("timezone", func(t *testing.T) {
		v := validCelebrationConfig()
		v.Timezone = "Mars/Olympus"
		if err := configValid(v); err == nil {
			t.Fatal("expected invalid timezone")
		}
	})
	t.Run("time", func(t *testing.T) {
		v := validCelebrationConfig()
		v.SendTime = "25:90"
		if err := configValid(v); err == nil {
			t.Fatal("expected invalid time")
		}
	})
	t.Run("template traversal", func(t *testing.T) {
		v := validCelebrationConfig()
		v.BirthdayTemplateKey = "../secret"
		if err := configValid(v); err == nil {
			t.Fatal("expected invalid template key")
		}
	})
	t.Run("retry bounds", func(t *testing.T) {
		v := validCelebrationConfig()
		v.MaxAttempts = 0
		if err := configValid(v); err == nil {
			t.Fatal("expected invalid attempts")
		}
	})
}

func TestCelebrationNextRunUsesCalendarDayAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	beforeDST := time.Date(2026, 3, 28, 10, 0, 0, 0, loc)
	next := nextCelebrationRun(beforeDST, "09:00")
	if next.Day() != 29 || next.Hour() != 9 {
		t.Fatalf("next = %v", next)
	}
}

func TestFeb29PolicyOnlyAppliesInNonLeapYears(t *testing.T) {
	utc := time.UTC
	if !shouldIncludeFeb29(time.Date(2025, 2, 28, 9, 0, 0, 0, utc), "feb28") {
		t.Fatal("expected Feb 28 policy")
	}
	if shouldIncludeFeb29(time.Date(2024, 2, 28, 9, 0, 0, 0, utc), "feb28") {
		t.Fatal("must not duplicate Feb 29 in leap year")
	}
	if !shouldIncludeFeb29(time.Date(2025, 3, 1, 9, 0, 0, 0, utc), "mar1") {
		t.Fatal("expected Mar 1 policy")
	}
	if shouldIncludeFeb29(time.Date(2025, 2, 28, 9, 0, 0, 0, utc), "leap_only") {
		t.Fatal("leap-only must not remap")
	}
}
