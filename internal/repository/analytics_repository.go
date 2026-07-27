package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

var ErrDuplicateAnalyticsBatch = errors.New("analytics batch already ingested")

// MonthlyEventStat is one month's event count, oldest first.
type MonthlyEventStat struct {
	Month string `json:"month"` // "YYYY-MM"
	Count int64  `json:"count"`
}

// AnalyticsRepository handles raw analytics data persistence and aggregation.
type AnalyticsRepository interface {
	EventSummary(ctx context.Context) (*EventSummary, error)
	CountEventsByCategory(ctx context.Context) (map[string]int64, error)
	EventsByMonth(ctx context.Context, months int) ([]MonthlyEventStat, error)
	OperationalSummary(ctx context.Context) (*OperationalSummary, error)
	IngestBatch(ctx context.Context, batch *models.AnalyticsBatch, events []models.AnalyticsEvent) error
}

type EventSummary struct {
	Total    int64
	Upcoming int64
}

type OperationalSummary struct {
	TotalMembers     int64 `json:"totalMembers"`
	ActiveMembers    int64 `json:"activeMembers"`
	TotalWorkforce   int64 `json:"totalWorkforce"`
	ServingWorkforce int64 `json:"servingWorkforce"`
	TotalSubmissions int64 `json:"totalSubmissions"`
	Submissions30d   int64 `json:"submissions30d"`
	TotalAttendance  int64 `json:"totalAttendance"`
	Attendance30d    int64 `json:"attendance30d"`
	ClientEvents30d  int64 `json:"clientEvents30d"`
}

type analyticsRepository struct {
	db *database.Database
}

func NewAnalyticsRepository(db *database.Database) AnalyticsRepository {
	return &analyticsRepository{db: db}
}

func (r *analyticsRepository) EventSummary(ctx context.Context) (*EventSummary, error) {
	var out EventSummary
	err := r.db.WithContext(ctx).Model(&models.Event{}).
		Select("COUNT(*) AS total, COUNT(*) FILTER (WHERE event_date >= CURRENT_DATE) AS upcoming").
		Scan(&out).Error
	return &out, err
}

func (r *analyticsRepository) CountEventsByCategory(ctx context.Context) (map[string]int64, error) {
	type catRow struct {
		Category string
		Count    int64
	}
	var rows []catRow
	if err := r.db.WithContext(ctx).Model(&models.Event{}).
		Select("category, COUNT(*) as count").
		Group("category").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Category] = row.Count
	}
	return out, nil
}

// EventsByMonth returns event counts for the last N months, oldest first,
// including months with zero events so charts don't show gaps. The "date"
// column is stored as validated YYYY-MM-DD text (see models.Event), so this
// only casts rows that actually match that shape — defensive against any
// legacy row that predates the format validation.
func (r *analyticsRepository) EventsByMonth(ctx context.Context, months int) ([]MonthlyEventStat, error) {
	if months <= 0 {
		months = 12
	}

	type row struct {
		Month string
		Count int64
	}
	// Window starts at the first day of the month N-1 months ago, so the
	// oldest bucket captures its whole calendar month rather than being
	// cut short by a day-precise "N months back from today" boundary.
	var rows []row
	err := r.db.WithContext(ctx).Model(&models.Event{}).
		Select("to_char(event_date, 'YYYY-MM') as month, COUNT(*) as count").
		Where("event_date IS NOT NULL").
		Where("event_date >= (date_trunc('month', CURRENT_DATE) - (? * INTERVAL '1 month'))", months-1).
		Group("month").
		Order("month ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	byMonth := make(map[string]int64, len(rows))
	for _, r := range rows {
		byMonth[r.Month] = r.Count
	}

	// Fill in every month in the window, even ones with zero events.
	now := time.Now().UTC()
	out := make([]MonthlyEventStat, 0, months)
	for i := months - 1; i >= 0; i-- {
		key := now.AddDate(0, -i, 0).Format("2006-01")
		out = append(out, MonthlyEventStat{Month: key, Count: byMonth[key]})
	}

	return out, nil
}

func (r *analyticsRepository) OperationalSummary(ctx context.Context) (*OperationalSummary, error) {
	var out OperationalSummary
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM members WHERE deleted_at IS NULL) AS total_members,
			(SELECT COUNT(*) FROM members WHERE deleted_at IS NULL AND is_active = TRUE) AS active_members,
			(SELECT COUNT(*) FROM workforce_members WHERE deleted_at IS NULL) AS total_workforce,
			(SELECT COUNT(*) FROM workforce_members WHERE deleted_at IS NULL AND status = ?) AS serving_workforce,
			(SELECT COUNT(*) FROM form_submissions WHERE deleted_at IS NULL) AS total_submissions,
			(SELECT COUNT(*) FROM form_submissions WHERE deleted_at IS NULL AND created_at >= NOW() - INTERVAL '30 days') AS submissions30d,
			(SELECT COALESCE(SUM(GREATEST(s.head_count, COALESCE(rc.record_count, 0))), 0)
			 FROM attendance_sessions s
			 LEFT JOIN (SELECT session_id, COUNT(*) AS record_count FROM attendance_records WHERE deleted_at IS NULL GROUP BY session_id) rc ON rc.session_id = s.id
			 WHERE s.deleted_at IS NULL) AS total_attendance,
			(SELECT COALESCE(SUM(GREATEST(s.head_count, COALESCE(rc.record_count, 0))), 0)
			 FROM attendance_sessions s
			 LEFT JOIN (SELECT session_id, COUNT(*) AS record_count FROM attendance_records WHERE deleted_at IS NULL GROUP BY session_id) rc ON rc.session_id = s.id
			 WHERE s.deleted_at IS NULL AND s.date >= NOW() - INTERVAL '30 days') AS attendance30d,
			(SELECT COUNT(*) FROM analytics_events WHERE occurred_at >= NOW() - INTERVAL '30 days') AS client_events30d
	`, models.WorkforceStatusServing).Scan(&out).Error
	return &out, err
}

func (r *analyticsRepository) IngestBatch(ctx context.Context, batch *models.AnalyticsBatch, events []models.AnalyticsEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "batch_id"}}, DoNothing: true}).Create(batch)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrDuplicateAnalyticsBatch
		}
		if len(events) == 0 {
			return nil
		}
		return tx.CreateInBatches(events, 100).Error
	})
}
