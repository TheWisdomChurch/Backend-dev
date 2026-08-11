package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	applog "wisdomHouse-backend/internal/logger"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/handlers"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
)

func verifyDatabaseConnection(db *database.Database) error {
	if db == nil {
		return errors.New("database is nil")
	}
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

type redisLock struct {
	client *redis.Client
}

func newRedisLock(redisURL string) *redisLock {
	if strings.TrimSpace(redisURL) == "" {
		return nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil
	}
	return &redisLock{client: redis.NewClient(opts)}
}

func (l *redisLock) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if l == nil || l.client == nil {
		return true, nil
	}
	return l.client.SetNX(ctx, key, "1", ttl).Result()
}

func startFormCleanup(ctx context.Context, svc service.FormService, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := svc.CleanupExpiredForms(time.Now().UTC())
			if err != nil {
				applog.L().Warn("form cleanup failed", "error", err)
				continue
			}
			if count > 0 {
				applog.L().Info("cleaned up expired forms", "count", count)
			}
		}
	}
}

func startAnalyticsRawCleanup(ctx context.Context, lock *redisLock, db *database.Database, interval time.Duration) {
	if db == nil {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	run := func() {
		if lock != nil {
			ok, err := lock.Acquire(ctx, "analytics_raw_cleanup:"+time.Now().UTC().Format("20060102"), 25*time.Hour)
			if err != nil || !ok {
				if err != nil {
					applog.L().Warn("analytics cleanup lock failed", "error", err)
				}
				return
			}
		}
		result := db.WithContext(ctx).Where("expires_at < ?", time.Now().UTC()).Delete(&models.AnalyticsBatch{})
		if result.Error != nil {
			applog.L().Warn("analytics raw batch cleanup failed", "error", result.Error)
		} else if result.RowsAffected > 0 {
			applog.L().Info("expired analytics raw batches removed", "count", result.RowsAffected)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func startFormReminderScheduler(
	ctx context.Context,
	lock *redisLock,
	svc service.FormService,
	interval time.Duration,
	lookAhead time.Duration,
) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}
	if lookAhead <= 0 {
		lookAhead = 24 * time.Hour
	}

	run := func() {
		now := time.Now().UTC()
		if lock != nil {
			key := "form_event_reminder:" + now.Format("2006010215")
			ok, err := lock.Acquire(ctx, key, 70*time.Minute)
			if err != nil {
				applog.L().Warn("form reminder lock failed", "error", err)
				return
			}
			if !ok {
				return
			}
		}

		sent, failed, err := svc.SendEventReminderEmails(now, lookAhead)
		if err != nil {
			applog.L().Warn("form reminder scheduler failed", "error", err)
			return
		}
		if sent > 0 || failed > 0 {
			applog.L().Info("form reminders sent", "sent", sent, "failed", failed)
		}
	}

	run()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func startNewMemberWorkflowScheduler(ctx context.Context, lock *redisLock, svc service.NewMemberWorkflowService, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	run := func() {
		now := time.Now().UTC()
		if lock != nil {
			ok, err := lock.Acquire(ctx, "new_member_workflow:"+now.Format("200601021504"), interval+time.Minute)
			if err != nil {
				applog.L().Warn("new-member workflow lock failed", "error", err)
				return
			}
			if !ok {
				return
			}
		}
		processed, err := svc.ProcessDue(ctx, now)
		if err != nil {
			applog.L().Warn("new-member workflow scheduler failed", "error", err)
			return
		}
		if processed > 0 {
			applog.L().Info("new-member workflow reminders processed", "count", processed)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func startVisitReminderScheduler(ctx context.Context, lock *redisLock, handler *handlers.EngagementHandler, interval time.Duration) {
	if handler == nil {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	run := func() {
		now := time.Now().UTC()
		if lock != nil {
			ok, err := lock.Acquire(ctx, "visit_reminders:"+now.Format("200601021504"), interval+time.Minute)
			if err != nil || !ok {
				return
			}
		}
		processed, err := handler.ProcessVisitReminders(ctx, now)
		if err != nil {
			applog.L().Warn("visit reminder scheduler failed", "error", err)
			return
		}
		if processed > 0 {
			applog.L().Info("visit reminders sent", "count", processed)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func parseHourMinute(raw string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time must be HH:MM")
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("hour must be numeric")
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("minute must be numeric")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("time must be valid 24h clock")
	}
	return hour, minute, nil
}

func nextRunAt(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func startBirthdayScheduler(
	ctx context.Context,
	lock *redisLock,
	workforceSvc service.WorkforceService,
	memberSvc service.MemberService,
	leadershipSvc service.LeadershipService,
	tz string,
	sendAt string,
) {
	if workforceSvc == nil && memberSvc == nil && leadershipSvc == nil {
		return
	}

	if strings.TrimSpace(sendAt) == "" {
		sendAt = "09:00"
	}
	hour, minute, err := parseHourMinute(sendAt)
	if err != nil {
		applog.L().Warn("invalid BIRTHDAY_SCHEDULER_TIME, using 09:00", "value", sendAt)
		hour, minute = 9, 0
	}

	loc := time.UTC
	if strings.TrimSpace(tz) != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		} else {
			applog.L().Warn("invalid BIRTHDAY_SCHEDULER_TZ, using UTC", "value", tz)
		}
	}

	for {
		now := time.Now().In(loc)
		next := nextRunAt(now, hour, minute)
		timer := time.NewTimer(time.Until(next))

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		dateKey := next.Format("2006-01-02")
		if lock != nil {
			ok, err := lock.Acquire(ctx, "birthday_send:"+dateKey, 36*time.Hour)
			if err != nil {
				applog.L().Warn("birthday scheduler lock failed", "error", err)
				continue
			}
			if !ok {
				applog.L().Info("birthday scheduler already ran", "date", dateKey)
				continue
			}
		}

		if workforceSvc != nil {
			result, err := workforceSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				applog.L().Warn("workforce birthday send failed", "error", err)
			} else {
				applog.L().Info("workforce birthdays sent", "targeted", result.Targeted, "sent", result.Sent, "skipped", result.Skipped)
			}
		}

		if memberSvc != nil {
			result, err := memberSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				applog.L().Warn("member birthday send failed", "error", err)
			} else {
				applog.L().Info("member birthdays sent", "targeted", result.Targeted, "sent", result.Sent, "skipped", result.Skipped)
			}
		}

		if leadershipSvc != nil {
			result, err := leadershipSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				applog.L().Warn("leadership birthday send failed", "error", err)
			} else {
				applog.L().Info("leadership birthdays sent", "targeted", result.Targeted, "sent", result.Sent, "skipped", result.Skipped)
			}

			result, err = leadershipSvc.SendAnniversaryGreetings(int(next.Month()), next.Day())
			if err != nil {
				applog.L().Warn("leadership anniversary send failed", "error", err)
			} else {
				applog.L().Info("leadership anniversaries sent", "targeted", result.Targeted, "sent", result.Sent, "skipped", result.Skipped)
			}
		}
	}
}
