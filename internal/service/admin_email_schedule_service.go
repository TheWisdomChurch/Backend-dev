package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type AdminEmailScheduleService interface {
	Create(*models.UpsertAdminEmailScheduleRequest, *models.AdminEmailSendActor) (*models.AdminEmailScheduleDetail, error)
	Update(string, *models.UpsertAdminEmailScheduleRequest) (*models.AdminEmailScheduleDetail, error)
	Get(string) (*models.AdminEmailScheduleDetail, error)
	List(int, int, string) ([]models.AdminEmailSchedule, int64, error)
	SetStatus(string, models.AdminEmailScheduleStatus) (*models.AdminEmailScheduleDetail, error)
	Delete(string) error
	ListRuns(string, int) ([]models.AdminEmailScheduleRun, error)
	ProcessDue(context.Context, time.Time, string, int) (int, error)
}

type adminEmailScheduleService struct {
	repo   repository.AdminEmailScheduleRepository
	mailer AdminEmailService
}

func NewAdminEmailScheduleService(repo repository.AdminEmailScheduleRepository, mailer AdminEmailService) AdminEmailScheduleService {
	return &adminEmailScheduleService{repo: repo, mailer: mailer}
}

func (s *adminEmailScheduleService) Create(req *models.UpsertAdminEmailScheduleRequest, actor *models.AdminEmailSendActor) (*models.AdminEmailScheduleDetail, error) {
	row, err := buildSchedule(req, nil, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if actor != nil {
		if actor.UserID != "" {
			row.CreatedByUserID = &actor.UserID
		}
		if actor.Email != "" {
			row.CreatedByEmail = &actor.Email
		}
		if actor.Role != "" {
			row.CreatedByRole = &actor.Role
		}
	}
	if err := s.repo.Create(row); err != nil {
		return nil, err
	}
	return scheduleDetail(row)
}

func (s *adminEmailScheduleService) Update(id string, req *models.UpsertAdminEmailScheduleRequest) (*models.AdminEmailScheduleDetail, error) {
	current, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if current.ClaimedAt != nil {
		return nil, errors.New("schedule is currently being processed")
	}
	row, err := buildSchedule(req, current, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(row); err != nil {
		return nil, err
	}
	return scheduleDetail(row)
}

func (s *adminEmailScheduleService) Get(id string) (*models.AdminEmailScheduleDetail, error) {
	row, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	return scheduleDetail(row)
}
func (s *adminEmailScheduleService) List(page, limit int, status string) ([]models.AdminEmailSchedule, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.List((page-1)*limit, limit, strings.TrimSpace(status))
}
func (s *adminEmailScheduleService) Delete(id string) error { return s.repo.Delete(id) }
func (s *adminEmailScheduleService) ListRuns(id string, limit int) ([]models.AdminEmailScheduleRun, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.ListRuns(id, limit)
}
func (s *adminEmailScheduleService) SetStatus(id string, status models.AdminEmailScheduleStatus) (*models.AdminEmailScheduleDetail, error) {
	row, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	switch status {
	case models.AdminEmailScheduleActive:
		next, calcErr := nextScheduleRun(row, time.Now().UTC().Add(-time.Second))
		if calcErr != nil {
			return nil, calcErr
		}
		if next == nil {
			return nil, errors.New("schedule has no future occurrence")
		}
		row.NextRunAt, row.Status, row.LastError, row.ConsecutiveErrors = next, status, nil, 0
	case models.AdminEmailSchedulePaused, models.AdminEmailScheduleDraft:
		row.Status, row.NextRunAt, row.ClaimedAt, row.ClaimedBy = status, nil, nil, nil
	default:
		return nil, errors.New("status must be active, paused, or draft")
	}
	if err := s.repo.Update(row); err != nil {
		return nil, err
	}
	return scheduleDetail(row)
}

func buildSchedule(req *models.UpsertAdminEmailScheduleRequest, current *models.AdminEmailSchedule, now time.Time) (*models.AdminEmailSchedule, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	if name == "" || utf8.RuneCountInString(name) > 160 {
		return nil, errors.New("name is required and must be 160 characters or fewer")
	}
	if utf8.RuneCountInString(description) > 500 {
		return nil, errors.New("description must be 500 characters or fewer")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(req.Timezone)); err != nil {
		return nil, errors.New("timezone must be a valid IANA timezone")
	}
	if _, err := time.Parse("15:04", req.SendTime); err != nil {
		return nil, errors.New("sendTime must use HH:mm (24-hour) format")
	}
	if req.EndAt != nil && !req.EndAt.After(req.StartAt) {
		return nil, errors.New("endAt must be after startAt")
	}
	if _, err := normalizeAdminComposeRequest(&req.Compose); err != nil {
		return nil, fmt.Errorf("compose: %w", err)
	}
	weekdays := uniqueInts(req.Weekdays)
	monthDays := uniqueInts(req.MonthDays)
	switch req.Recurrence {
	case models.AdminEmailRecurrenceOnce:
		weekdays, monthDays = nil, nil
	case models.AdminEmailRecurrenceWeekly:
		if len(weekdays) == 0 {
			return nil, errors.New("weekly schedules require at least one weekday")
		}
		for _, day := range weekdays {
			if day < 0 || day > 6 {
				return nil, errors.New("weekdays must be between 0 (Sunday) and 6 (Saturday)")
			}
		}
		monthDays = nil
	case models.AdminEmailRecurrenceMonthly:
		if len(monthDays) == 0 {
			return nil, errors.New("monthly schedules require at least one month day")
		}
		for _, day := range monthDays {
			if day < 1 || day > 31 {
				return nil, errors.New("monthDays must be between 1 and 31")
			}
		}
		weekdays = nil
	default:
		return nil, errors.New("recurrence must be once, weekly, or monthly")
	}
	payload, _ := json.Marshal(req.Compose)
	weekdayJSON, _ := json.Marshal(weekdays)
	monthJSON, _ := json.Marshal(monthDays)
	status := req.Status
	if status == "" {
		status = models.AdminEmailScheduleDraft
	}
	if status != models.AdminEmailScheduleDraft && status != models.AdminEmailScheduleActive && status != models.AdminEmailSchedulePaused {
		return nil, errors.New("status must be draft, active, or paused")
	}
	row := &models.AdminEmailSchedule{Name: name, Description: description, Status: status, Recurrence: req.Recurrence, Timezone: req.Timezone, SendTime: req.SendTime, Weekdays: datatypes.JSON(weekdayJSON), MonthDays: datatypes.JSON(monthJSON), StartAt: req.StartAt.UTC(), EndAt: req.EndAt, ComposePayload: datatypes.JSON(payload), Subject: strings.TrimSpace(valueOrEmpty(req.Compose.Subject)), AudienceLabel: strings.TrimSpace(req.AudienceLabel)}
	if current != nil {
		row.ID, row.CreatedAt, row.CreatedByUserID, row.CreatedByEmail, row.CreatedByRole = current.ID, current.CreatedAt, current.CreatedByUserID, current.CreatedByEmail, current.CreatedByRole
		row.RunCount, row.LastRunAt = current.RunCount, current.LastRunAt
	}
	if status == models.AdminEmailScheduleActive {
		next, err := nextScheduleRun(row, now.Add(-time.Second))
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, errors.New("schedule has no future occurrence")
		}
		row.NextRunAt = next
	}
	return row, nil
}

func scheduleDetail(row *models.AdminEmailSchedule) (*models.AdminEmailScheduleDetail, error) {
	var compose models.SendAdminComposeEmailRequest
	if err := json.Unmarshal(row.ComposePayload, &compose); err != nil {
		return nil, err
	}
	return &models.AdminEmailScheduleDetail{AdminEmailSchedule: *row, Compose: compose}, nil
}
func uniqueInts(values []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func nextScheduleRun(row *models.AdminEmailSchedule, after time.Time) (*time.Time, error) {
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		return nil, err
	}
	if row.Recurrence == models.AdminEmailRecurrenceOnce {
		startLocal := row.StartAt.In(loc)
		clock, _ := time.Parse("15:04", row.SendTime)
		candidate := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), clock.Hour(), clock.Minute(), 0, 0, loc).UTC()
		if candidate.After(after) && (row.EndAt == nil || !candidate.After(*row.EndAt)) {
			return &candidate, nil
		}
		return nil, nil
	}
	var weekdays, monthDays []int
	_ = json.Unmarshal(row.Weekdays, &weekdays)
	_ = json.Unmarshal(row.MonthDays, &monthDays)
	clock, _ := time.Parse("15:04", row.SendTime)
	cursor := after.In(loc)
	startLocal := row.StartAt.In(loc)
	startDate := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc)
	for offset := 0; offset < 800; offset++ {
		date := time.Date(cursor.Year(), cursor.Month(), cursor.Day()+offset, clock.Hour(), clock.Minute(), 0, 0, loc)
		if date.Before(startDate) || !date.UTC().After(after) {
			continue
		}
		if row.EndAt != nil && date.UTC().After(row.EndAt.UTC()) {
			return nil, nil
		}
		match := false
		if row.Recurrence == models.AdminEmailRecurrenceWeekly {
			for _, d := range weekdays {
				if int(date.Weekday()) == d {
					match = true
					break
				}
			}
		}
		if row.Recurrence == models.AdminEmailRecurrenceMonthly {
			for _, d := range monthDays {
				if date.Day() == d {
					match = true
					break
				}
			}
		}
		if match {
			value := date.UTC()
			return &value, nil
		}
	}
	return nil, errors.New("could not calculate next occurrence")
}

func (s *adminEmailScheduleService) ProcessDue(ctx context.Context, now time.Time, worker string, limit int) (int, error) {
	rows, err := s.repo.ClaimDue(ctx, now.UTC(), worker, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for i := range rows {
		if ctx.Err() != nil {
			break
		}
		s.processOne(&rows[i], now.UTC())
		processed++
	}
	return processed, nil
}
func (s *adminEmailScheduleService) processOne(row *models.AdminEmailSchedule, now time.Time) {
	scheduledFor := *row.NextRunAt
	run := &models.AdminEmailScheduleRun{ScheduleID: row.ID, ScheduledFor: scheduledFor, Status: "running", StartedAt: now}
	if err := s.repo.CreateRun(run); err != nil {
		_ = s.repo.ReleaseClaim(row.ID)
		return
	}
	var req models.SendAdminComposeEmailRequest
	err := json.Unmarshal(row.ComposePayload, &req)
	var result *models.SendAdminComposeEmailResponse
	if err == nil {
		result, err = s.mailer.SendComposeEmail(&req, &models.AdminEmailSendActor{UserID: valueOrEmpty(row.CreatedByUserID), Email: valueOrEmpty(row.CreatedByEmail), Role: valueOrEmpty(row.CreatedByRole)})
	}
	completed := time.Now().UTC()
	run.CompletedAt = &completed
	row.LastRunAt = &scheduledFor
	row.RunCount++
	row.ClaimedAt = nil
	row.ClaimedBy = nil
	if err != nil {
		message := err.Error()
		run.Status = "failed"
		run.Error = &message
		row.LastError = &message
		row.ConsecutiveErrors++
		// Three consecutive permanent failures pause the schedule to prevent an unbounded retry storm.
		if row.ConsecutiveErrors >= 3 {
			row.Status = models.AdminEmailScheduleFailed
			row.NextRunAt = nil
		} else {
			retry := now.Add(time.Duration(row.ConsecutiveErrors) * 15 * time.Minute)
			row.NextRunAt = &retry
		}
	} else {
		run.Status = "completed"
		run.Sent = result.Sent
		run.Failed = result.Failed
		run.DeliveryID = result.DeliveryID
		row.LastError = nil
		row.ConsecutiveErrors = 0
		next, _ := nextScheduleRun(row, scheduledFor)
		row.NextRunAt = next
		if next == nil {
			row.Status = models.AdminEmailScheduleCompleted
		}
	}
	_ = s.repo.CompleteRun(run, row)
}
