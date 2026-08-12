package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
)

type scheduleRepoStub struct {
	completedRun      *models.AdminEmailScheduleRun
	completedSchedule *models.AdminEmailSchedule
}

func (*scheduleRepoStub) Create(*models.AdminEmailSchedule) error { return nil }
func (*scheduleRepoStub) Update(*models.AdminEmailSchedule) error { return nil }
func (*scheduleRepoStub) Get(string) (*models.AdminEmailSchedule, error) {
	return nil, errors.New("not implemented")
}
func (*scheduleRepoStub) List(int, int, string) ([]models.AdminEmailSchedule, int64, error) {
	return nil, 0, nil
}
func (*scheduleRepoStub) Delete(string) error { return nil }
func (*scheduleRepoStub) ClaimDue(context.Context, time.Time, string, int) ([]models.AdminEmailSchedule, error) {
	return nil, nil
}
func (*scheduleRepoStub) RenewClaim(string, string, time.Time) (bool, error) { return true, nil }
func (*scheduleRepoStub) ReleaseClaim(string, string) error                  { return nil }
func (*scheduleRepoStub) CreateRun(*models.AdminEmailScheduleRun) error      { return nil }
func (r *scheduleRepoStub) CompleteRun(run *models.AdminEmailScheduleRun, schedule *models.AdminEmailSchedule) error {
	copyRun := *run
	copySchedule := *schedule
	r.completedRun = &copyRun
	r.completedSchedule = &copySchedule
	return nil
}
func (*scheduleRepoStub) ListRuns(string, int) ([]models.AdminEmailScheduleRun, error) {
	return nil, nil
}

type adminMailerStub struct {
	result *models.SendAdminComposeEmailResponse
	err    error
}

func (m adminMailerStub) SendComposeEmail(*models.SendAdminComposeEmailRequest, *models.AdminEmailSendActor) (*models.SendAdminComposeEmailResponse, error) {
	return m.result, m.err
}
func (adminMailerStub) ListDeliveries(int, int) ([]models.AdminEmailDeliveryHistoryItem, int64, error) {
	return nil, 0, nil
}
func (adminMailerStub) GetMarketingSummary() (*models.AdminEmailMarketingSummary, error) {
	return nil, nil
}
func (adminMailerStub) ListAudienceForms(int, int) ([]models.AdminEmailMarketingFormItem, int64, error) {
	return nil, 0, nil
}
func (adminMailerStub) PreviewAudience([]string, []string, int) (*models.AdminEmailAudiencePreview, error) {
	return nil, nil
}

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

func claimedWeeklySchedule(now time.Time) *models.AdminEmailSchedule {
	weekdays, _ := json.Marshal([]int{int(now.Weekday())})
	worker := "worker-test"
	claimed := now
	return &models.AdminEmailSchedule{ID: "schedule-1", Status: models.AdminEmailScheduleActive, Recurrence: models.AdminEmailRecurrenceWeekly, Timezone: "UTC", SendTime: now.Format("15:04"), StartDate: now.AddDate(0, 0, -1).Format("2006-01-02"), Weekdays: weekdays, NextRunAt: &now, ClaimedAt: &claimed, ClaimedBy: &worker, ComposePayload: []byte(`{}`)}
}

func TestProcessOneRetriesSameOccurrenceWhenAllDeliveriesFail(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	row := claimedWeeklySchedule(now)
	repo := &scheduleRepoStub{}
	svc := &adminEmailScheduleService{repo: repo, mailer: adminMailerStub{result: &models.SendAdminComposeEmailResponse{Failed: 2}}}
	if err := svc.processOne(context.Background(), row, now); err != nil {
		t.Fatal(err)
	}
	if repo.completedRun == nil || repo.completedRun.Status != "failed" {
		t.Fatalf("unexpected run: %#v", repo.completedRun)
	}
	if repo.completedSchedule.PendingOccurrenceAt == nil || !repo.completedSchedule.PendingOccurrenceAt.Equal(now) {
		t.Fatalf("retry lost original occurrence: %#v", repo.completedSchedule.PendingOccurrenceAt)
	}
	if repo.completedSchedule.RunCount != 0 {
		t.Fatalf("failed attempt counted as completed run")
	}
	wantRetry := now.Add(15 * time.Minute)
	if repo.completedSchedule.NextRunAt == nil || !repo.completedSchedule.NextRunAt.Equal(wantRetry) {
		t.Fatalf("retry = %v, want %v", repo.completedSchedule.NextRunAt, wantRetry)
	}
}

func TestProcessOneDoesNotResendPartialDelivery(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	row := claimedWeeklySchedule(now)
	repo := &scheduleRepoStub{}
	svc := &adminEmailScheduleService{repo: repo, mailer: adminMailerStub{result: &models.SendAdminComposeEmailResponse{Sent: 3, Failed: 1}}}
	if err := svc.processOne(context.Background(), row, now); err != nil {
		t.Fatal(err)
	}
	if repo.completedRun.Status != "partial" {
		t.Fatalf("status = %s", repo.completedRun.Status)
	}
	if repo.completedSchedule.PendingOccurrenceAt != nil {
		t.Fatal("partial delivery must not retry the whole audience")
	}
	if repo.completedSchedule.RunCount != 1 {
		t.Fatalf("run count = %d", repo.completedSchedule.RunCount)
	}
}

func TestProcessOneEmptySuppressedAudienceAdvancesWithoutFailure(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	row := claimedWeeklySchedule(now)
	repo := &scheduleRepoStub{}
	svc := &adminEmailScheduleService{repo: repo, mailer: adminMailerStub{err: fmt.Errorf("%w: opted out", ErrNoDeliverableRecipients)}}
	if err := svc.processOne(context.Background(), row, now); err != nil {
		t.Fatal(err)
	}
	if repo.completedRun.Status != "completed" || repo.completedSchedule.ConsecutiveErrors != 0 {
		t.Fatalf("suppressed audience treated as failure: %#v", repo.completedSchedule)
	}
}

func TestNextRunAfterExecutionSkipsMissedRecurringBacklog(t *testing.T) {
	weekdays, _ := json.Marshal([]int{1})
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceWeekly, Timezone: "UTC", SendTime: "09:00", StartDate: "2026-01-01", Weekdays: weekdays}
	staleOccurrence := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	recoveredAt := time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC)
	next, err := nextRunAfterExecution(row, staleOccurrence, recoveredAt)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 2, 9, 9, 0, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}

func TestNextRunAfterExecutionStillCompletesStaleOneTimeSchedule(t *testing.T) {
	row := &models.AdminEmailSchedule{Recurrence: models.AdminEmailRecurrenceOnce, Timezone: "UTC", SendTime: "09:00", StartDate: "2026-01-05"}
	occurrence := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	next, err := nextRunAfterExecution(row, occurrence, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if next != nil {
		t.Fatalf("one-time schedule should complete, got %v", next)
	}
}
