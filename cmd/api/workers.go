package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"wisdomHouse-backend/internal/database"
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

func startFormCleanup(ctx context.Context, logger *log.Logger, svc service.FormService, interval time.Duration) {
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
				if logger != nil {
					logger.Printf("⚠️ Form cleanup failed: %v", err)
				}
				continue
			}
			if count > 0 && logger != nil {
				logger.Printf("🧹 Cleaned up %d expired forms", count)
			}
		}
	}
}

func startFormReminderScheduler(
	ctx context.Context,
	logger *log.Logger,
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
				if logger != nil {
					logger.Printf("⚠️ Form reminder lock failed: %v", err)
				}
				return
			}
			if !ok {
				return
			}
		}

		sent, failed, err := svc.SendEventReminderEmails(now, lookAhead)
		if err != nil {
			if logger != nil {
				logger.Printf("⚠️ Form reminder scheduler failed: %v", err)
			}
			return
		}
		if logger != nil && (sent > 0 || failed > 0) {
			logger.Printf("📩 Form reminders: sent=%d failed=%d", sent, failed)
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
	logger *log.Logger,
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
		if logger != nil {
			logger.Printf("⚠️ Invalid BIRTHDAY_SCHEDULER_TIME=%q, using 09:00", sendAt)
		}
		hour, minute = 9, 0
	}

	loc := time.UTC
	if strings.TrimSpace(tz) != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		} else if logger != nil {
			logger.Printf("⚠️ Invalid BIRTHDAY_SCHEDULER_TZ=%q, using UTC", tz)
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
				if logger != nil {
					logger.Printf("⚠️ Birthday scheduler lock failed: %v", err)
				}
				continue
			}
			if !ok {
				if logger != nil {
					logger.Printf("ℹ️ Birthday scheduler already ran for %s", dateKey)
				}
				continue
			}
		}

		if workforceSvc != nil {
			result, err := workforceSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Workforce birthday send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("🎂 Workforce birthdays: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}
		}

		if memberSvc != nil {
			result, err := memberSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Member birthday send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("🎉 Member birthdays: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}
		}

		if leadershipSvc != nil {
			result, err := leadershipSvc.SendBirthdayGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Leadership birthday send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("🎂 Leadership birthdays: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}

			result, err = leadershipSvc.SendAnniversaryGreetings(int(next.Month()), next.Day())
			if err != nil {
				if logger != nil {
					logger.Printf("⚠️ Leadership anniversary send failed: %v", err)
				}
			} else if logger != nil {
				logger.Printf("💍 Leadership anniversaries: targeted=%d sent=%d skipped=%d", result.Targeted, result.Sent, result.Skipped)
			}
		}
	}
}
