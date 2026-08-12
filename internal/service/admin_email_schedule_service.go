package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	if s == nil || s.repo == nil || s.mailer == nil {
		return nil, errors.New("email scheduler is not configured")
	}
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
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("schedule id is required")
	}
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
	status = strings.TrimSpace(status)
	if status != "" && status != string(models.AdminEmailScheduleDraft) && status != string(models.AdminEmailScheduleActive) && status != string(models.AdminEmailSchedulePaused) && status != string(models.AdminEmailScheduleCompleted) && status != string(models.AdminEmailScheduleFailed) {
		return nil, 0, errors.New("invalid schedule status filter")
	}
	return s.repo.List((page-1)*limit, limit, status)
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
		row.PendingOccurrenceAt = nil
		next, calcErr := nextScheduleRun(row, time.Now().UTC().Add(-time.Second))
		if calcErr != nil {
			return nil, calcErr
		}
		if next == nil {
			return nil, errors.New("schedule has no future occurrence")
		}
		row.NextRunAt, row.Status, row.LastError, row.ConsecutiveErrors = next, status, nil, 0
	case models.AdminEmailSchedulePaused, models.AdminEmailScheduleDraft:
		if row.ClaimedAt != nil {
			return nil, errors.New("schedule is currently being processed")
		}
		row.Status, row.NextRunAt, row.PendingOccurrenceAt = status, nil, nil
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
	timezone := strings.TrimSpace(req.Timezone)
	sendTime := strings.TrimSpace(req.SendTime)
	startDate := strings.TrimSpace(req.StartDate)
	if name == "" || utf8.RuneCountInString(name) > 160 {
		return nil, errors.New("name is required and must be 160 characters or fewer")
	}
	if utf8.RuneCountInString(description) > 500 {
		return nil, errors.New("description must be 500 characters or fewer")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, errors.New("timezone must be a valid IANA timezone")
	}
	if _, err := time.Parse("15:04", sendTime); err != nil {
		return nil, errors.New("sendTime must use HH:mm (24-hour) format")
	}
	startLocal, err := parseScheduleDate(startDate, loc)
	if err != nil {
		return nil, errors.New("startDate must use YYYY-MM-DD format")
	}
	var endDate *string
	var endAt *time.Time
	if req.EndDate != nil && strings.TrimSpace(*req.EndDate) != "" {
		value := strings.TrimSpace(*req.EndDate)
		parsedEnd, parseErr := parseScheduleDate(value, loc)
		if parseErr != nil {
			return nil, errors.New("endDate must use YYYY-MM-DD format")
		}
		if parsedEnd.Before(startLocal) {
			return nil, errors.New("endDate must be on or after startDate")
		}
		endDate = &value
		legacyEnd := parsedEnd.Add(24*time.Hour - time.Nanosecond).UTC()
		endAt = &legacyEnd
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
	if len(payload) > 2*1024*1024 {
		return nil, errors.New("compose payload exceeds the 2MB schedule limit")
	}
	weekdayJSON, _ := json.Marshal(weekdays)
	monthJSON, _ := json.Marshal(monthDays)
	status := req.Status
	if status == "" {
		status = models.AdminEmailScheduleDraft
	}
	if status != models.AdminEmailScheduleDraft && status != models.AdminEmailScheduleActive && status != models.AdminEmailSchedulePaused {
		return nil, errors.New("status must be draft, active, or paused")
	}
	audienceLabel := strings.TrimSpace(req.AudienceLabel)
	if utf8.RuneCountInString(audienceLabel) > 255 {
		return nil, errors.New("audienceLabel must be 255 characters or fewer")
	}
	// StartAt/EndAt remain populated for compatibility with installations that
	// applied migration 013. Recurrence uses the timezone-safe DATE columns.
	legacyStart := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 12, 0, 0, 0, time.UTC)
	row := &models.AdminEmailSchedule{Name: name, Description: description, Status: status, Recurrence: req.Recurrence, Timezone: timezone, SendTime: sendTime, StartDate: startDate, EndDate: endDate, Weekdays: datatypes.JSON(weekdayJSON), MonthDays: datatypes.JSON(monthJSON), StartAt: legacyStart, EndAt: endAt, ComposePayload: datatypes.JSON(payload), Subject: strings.TrimSpace(valueOrEmpty(req.Compose.Subject)), AudienceLabel: audienceLabel, Version: 1}
	if current != nil {
		row.ID, row.CreatedAt, row.CreatedByUserID, row.CreatedByEmail, row.CreatedByRole = current.ID, current.CreatedAt, current.CreatedByUserID, current.CreatedByEmail, current.CreatedByRole
		row.RunCount, row.LastRunAt = current.RunCount, current.LastRunAt
		row.Version = current.Version
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
	startLocal, err := parseScheduleDate(row.StartDate, loc)
	if err != nil {
		return nil, errors.New("schedule has an invalid start date")
	}
	var endLocal *time.Time
	if row.EndDate != nil {
		parsed, parseErr := parseScheduleDate(*row.EndDate, loc)
		if parseErr != nil {
			return nil, errors.New("schedule has an invalid end date")
		}
		endLocal = &parsed
	}
	clock, clockErr := time.Parse("15:04", row.SendTime)
	if clockErr != nil {
		return nil, clockErr
	}
	if row.Recurrence == models.AdminEmailRecurrenceOnce {
		candidate := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), clock.Hour(), clock.Minute(), 0, 0, loc).UTC()
		if candidate.After(after) && (endLocal == nil || !startLocal.After(*endLocal)) {
			return &candidate, nil
		}
		return nil, nil
	}
	var weekdays, monthDays []int
	_ = json.Unmarshal(row.Weekdays, &weekdays)
	_ = json.Unmarshal(row.MonthDays, &monthDays)
	cursor := after.In(loc)
	startDate := startLocal
	if cursor.Before(startDate) {
		cursor = startDate
	}
	for offset := 0; offset < 800; offset++ {
		date := time.Date(cursor.Year(), cursor.Month(), cursor.Day()+offset, clock.Hour(), clock.Minute(), 0, 0, loc)
		if date.Before(startDate) || !date.UTC().After(after) {
			continue
		}
		if endLocal != nil && time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc).After(*endLocal) {
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

func parseScheduleDate(value string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	return time.ParseInLocation("2006-01-02", strings.TrimSpace(value), loc)
}

func (s *adminEmailScheduleService) ProcessDue(ctx context.Context, now time.Time, worker string, limit int) (int, error) {
	if s == nil || s.repo == nil || s.mailer == nil {
		return 0, errors.New("email scheduler is not configured")
	}
	worker = strings.TrimSpace(worker)
	if worker == "" {
		return 0, errors.New("worker id is required")
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 25 {
		limit = 25
	}
	processed := 0
	var processErrors []error
	for processed < limit && ctx.Err() == nil {
		rows, err := s.repo.ClaimDue(ctx, now.UTC(), worker, 1)
		if err != nil {
			return processed, err
		}
		if len(rows) == 0 {
			break
		}
		if err := s.processOne(ctx, &rows[0], now.UTC()); err != nil {
			processErrors = append(processErrors, err)
		}
		processed++
	}
	return processed, errors.Join(processErrors...)
}
func (s *adminEmailScheduleService) processOne(ctx context.Context, row *models.AdminEmailSchedule, now time.Time) error {
	if row == nil || row.NextRunAt == nil || row.ClaimedBy == nil {
		return errors.New("claimed schedule is incomplete")
	}
	worker := *row.ClaimedBy
	scheduledFor := *row.NextRunAt
	if row.PendingOccurrenceAt != nil {
		scheduledFor = *row.PendingOccurrenceAt
	}
	run := &models.AdminEmailScheduleRun{ScheduleID: row.ID, ScheduledFor: scheduledFor, Status: "running", StartedAt: now}
	if err := s.repo.CreateRun(run); err != nil {
		_ = s.repo.ReleaseClaim(row.ID, worker)
		return fmt.Errorf("create schedule run: %w", err)
	}
	stopHeartbeat := make(chan struct{})
	var heartbeatWG sync.WaitGroup
	heartbeatWG.Add(1)
	go func() {
		defer heartbeatWG.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ctx.Done():
				return
			case tick := <-ticker.C:
				_, _ = s.repo.RenewClaim(row.ID, worker, tick.UTC())
			}
		}
	}()
	defer func() { close(stopHeartbeat); heartbeatWG.Wait() }()
	var req models.SendAdminComposeEmailRequest
	err := json.Unmarshal(row.ComposePayload, &req)
	var result *models.SendAdminComposeEmailResponse
	if err == nil {
		result, err = s.mailer.SendComposeEmail(&req, &models.AdminEmailSendActor{UserID: valueOrEmpty(row.CreatedByUserID), Email: valueOrEmpty(row.CreatedByEmail), Role: valueOrEmpty(row.CreatedByRole)})
	}
	if result != nil {
		run.Sent, run.Failed, run.DeliveryID = result.Sent, result.Failed, result.DeliveryID
		if err == nil && result.Sent == 0 && result.Failed > 0 {
			err = errors.New("all recipient deliveries failed")
		}
	} else if err == nil {
		err = errors.New("email delivery returned an empty result")
	}
	completed := time.Now().UTC()
	run.CompletedAt = &completed
	row.LastRunAt = &scheduledFor
	if errors.Is(err, ErrNoDeliverableRecipients) {
		run.Status = "completed"
		row.RunCount++
		row.PendingOccurrenceAt, row.LastError, row.NextRunAt = nil, nil, nil
		row.ConsecutiveErrors = 0
		next, nextErr := nextRunAfterExecution(row, scheduledFor, completed)
		if nextErr != nil {
			message := nextErr.Error()
			row.Status = models.AdminEmailScheduleFailed
			row.LastError = &message
			if completeErr := s.repo.CompleteRun(run, row); completeErr != nil {
				return fmt.Errorf("complete empty-audience run after recurrence error: %w", completeErr)
			}
			return fmt.Errorf("calculate next empty-audience occurrence: %w", nextErr)
		}
		row.NextRunAt = next
		if next == nil {
			row.Status = models.AdminEmailScheduleCompleted
		}
	} else if err != nil {
		message := err.Error()
		run.Status = "failed"
		run.Error = &message
		row.LastError = &message
		row.ConsecutiveErrors++
		// Three consecutive failed attempts suspend the schedule to prevent an unbounded retry storm.
		row.PendingOccurrenceAt = &scheduledFor
		if row.ConsecutiveErrors >= 3 {
			row.Status = models.AdminEmailScheduleFailed
			row.NextRunAt = nil
		} else {
			retry := now.Add(time.Duration(row.ConsecutiveErrors) * 15 * time.Minute)
			row.NextRunAt = &retry
		}
	} else {
		row.RunCount++
		run.Status = "completed"
		if result.Failed > 0 {
			run.Status = "partial"
			warning := fmt.Sprintf("%d recipient deliveries failed; see delivery history", result.Failed)
			row.LastError = &warning
		}
		if result.Failed == 0 {
			row.LastError = nil
		}
		row.ConsecutiveErrors = 0
		row.PendingOccurrenceAt = nil
		next, nextErr := nextRunAfterExecution(row, scheduledFor, completed)
		if nextErr != nil {
			message := nextErr.Error()
			row.Status = models.AdminEmailScheduleFailed
			row.LastError = &message
			row.NextRunAt = nil
			if completeErr := s.repo.CompleteRun(run, row); completeErr != nil {
				return fmt.Errorf("complete schedule run after recurrence error: %w", completeErr)
			}
			return fmt.Errorf("calculate next schedule occurrence: %w", nextErr)
		}
		row.NextRunAt = next
		if next == nil {
			row.Status = models.AdminEmailScheduleCompleted
		}
	}
	if err := s.repo.CompleteRun(run, row); err != nil {
		return fmt.Errorf("complete schedule run: %w", err)
	}
	return nil
}

func nextRunAfterExecution(row *models.AdminEmailSchedule, scheduledFor, completedAt time.Time) (*time.Time, error) {
	after := scheduledFor
	// Recurring campaigns use a skip-missed-runs policy. After an outage we
	// deliver the oldest claimed occurrence once, then jump to the next future
	// slot instead of blasting every historical weekly/monthly occurrence.
	if row != nil && row.Recurrence != models.AdminEmailRecurrenceOnce && completedAt.After(after) {
		after = completedAt
	}
	return nextScheduleRun(row, after)
}
