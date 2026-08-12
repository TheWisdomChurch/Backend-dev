package service

import (
	"encoding/json"
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
)

func TestNextScheduleRunWeeklyInTimezone(t *testing.T) {
	weekdays, _ := json.Marshal([]int{0, 3})
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceWeekly, Timezone: "Africa/Lagos", SendTime: "09:30", Weekdays: weekdays, StartAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
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
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceMonthly, Timezone: "UTC", SendTime: "08:00", MonthDays: monthDays, StartAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
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
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceOnce, Timezone: "Africa/Lagos", SendTime: "09:00", StartAt: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)}
	next, err := nextScheduleRun(row, time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}
