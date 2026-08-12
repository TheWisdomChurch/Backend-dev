package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	applog "wisdomHouse-backend/internal/logger"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/handlers"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
)

func startAdminEmailScheduleWorker(ctx context.Context, svc service.AdminEmailScheduleService, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	worker := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if worker == "" {
		worker = "email-scheduler"
	}
	run := func() {
		processed, err := svc.ProcessDue(ctx, time.Now().UTC(), worker, 10)
		if err != nil {
			applog.L().Error("email schedule worker failed", "error", err)
			return
		}
		if processed > 0 {
			applog.L().Info("email schedules processed", "count", processed)
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

func startStoreReservationScheduler(ctx context.Context, lock *redisLock, svc service.StoreService, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	run := func() {
		now := time.Now().UTC()
		if lock != nil {
			ok, err := lock.Acquire(ctx, "store:expire-reservations", interval)
			if err != nil || !ok {
				return
			}
		}
		count, err := svc.ExpirePendingReservations(now, 250)
		if err != nil {
			applog.L().Warn("store reservation cleanup failed", "error", err)
			return
		}
		if count > 0 {
			applog.L().Info("expired abandoned store reservations", "count", count)
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

func startCelebrationAutomationWorker(ctx context.Context, svc service.CelebrationAutomationService, interval time.Duration) {
	if svc == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	worker := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if worker == "" {
		worker = "celebration-worker"
	}
	run := func() {
		result, err := svc.ProcessDue(ctx, time.Now(), worker, "scheduler")
		if err != nil {
			applog.L().Error("celebration automation worker failed", "error", err)
			return
		}
		if result != nil {
			applog.L().Info("celebration automation processed", "date", result.RunDate, "status", result.Status, "sent", result.Sent, "failed", result.Failed, "suppressed", result.Suppressed)
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
