package service

import (
	"encoding/json"
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
)

func TestNextScheduleRunWeeklyInTimezone(t *testing.T) {
	weekdays, _ := json.Marshal([]int{0, 3})
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceWeekly, Timezone: "Africa/Lagos", SendTime: "09:30", StartDate: "2026-08-01", Weekdays: weekdays, StartAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	next, err := nextScheduleRun(row, time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 12, 8, 30, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}

func TestNextScheduleRunMonthlySkipsMissingDay(t *testing.T) {
	monthDays, _ := json.Marshal([]int{31})
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceMonthly, Timezone: "UTC", SendTime: "08:00", StartDate: "2026-04-01", MonthDays: monthDays, StartAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	next, err := nextScheduleRun(row, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}

func TestNextScheduleRunOnceUsesConfiguredLocalTime(t *testing.T) {
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceOnce, Timezone: "Africa/Lagos", SendTime: "09:00", StartDate: "2026-08-20", StartAt: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)}
	next, err := nextScheduleRun(row, time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}

func TestNextScheduleRunPreservesDateAtExtremePositiveOffset(t *testing.T) {
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceOnce, Timezone: "Pacific/Kiritimati", SendTime: "07:15", StartDate: "2026-01-02"}
	next, err := nextScheduleRun(row, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 1, 17, 15, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}

func TestNextScheduleRunHandlesFutureStartBeyondOldScanWindow(t *testing.T) {
	weekdays, _ := json.Marshal([]int{1})
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceWeekly, Timezone: "UTC", SendTime: "10:00", StartDate: "2035-01-01", Weekdays: weekdays}
	next, err := nextScheduleRun(row, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.Before(time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected next occurrence: %v", next)
	}
}

func TestNextScheduleRunHonorsInclusiveEndDate(t *testing.T) {
	weekdays, _ := json.Marshal([]int{3})
	end := "2026-08-12"
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceWeekly, Timezone: "Africa/Lagos", SendTime: "23:30", StartDate: "2026-08-01", EndDate: &end, Weekdays: weekdays}
	next, err := nextScheduleRun(row, time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 12, 22, 30, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
	after, err := nextScheduleRun(row, *next)
	if err != nil {
		t.Fatal(err)
	}
	if after != nil {
		t.Fatalf("expected no occurrence after end date, got %v", after)
	}
}

func TestParseScheduleDateRejectsImpossibleDate(t *testing.T) {
	if _, err := parseScheduleDate("2026-02-30", time.UTC); err == nil {
		t.Fatal("expected invalid calendar date to fail")
	}
}
