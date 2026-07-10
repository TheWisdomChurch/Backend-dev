package repository

import (
	"context"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

// MonthlyEventStat is one month's event count, oldest first.
type MonthlyEventStat struct {
	Month string `json:"month"` // "YYYY-MM"
	Count int64  `json:"count"`
}

// AnalyticsRepository handles raw analytics data persistence and aggregation.
type AnalyticsRepository interface {
	CountAllEvents(ctx context.Context) (int64, error)
	CountUpcomingEvents(ctx context.Context) (int64, error)
	SumAttendees(ctx context.Context) (int64, error)
	CountEventsByCategory(ctx context.Context) (map[string]int64, error)
	EventsByMonth(ctx context.Context, months int) ([]MonthlyEventStat, error)
	IngestBatch(ctx context.Context, batch *models.AnalyticsBatch) error
}

type analyticsRepository struct {
	db *database.Database
}

func NewAnalyticsRepository(db *database.Database) AnalyticsRepository {
	return &analyticsRepository{db: db}
}

func (r *analyticsRepository) CountAllEvents(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Event{}).Count(&n).Error
	return n, err
}

func (r *analyticsRepository) CountUpcomingEvents(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Event{}).
		Where("status = ?", models.EventStatusUpcoming).
		Count(&n).Error
	return n, err
}

func (r *analyticsRepository) SumAttendees(ctx context.Context) (int64, error) {
	type row struct{ Sum int64 }
	var r2 row
	err := r.db.WithContext(ctx).Model(&models.Event{}).
		Select("COALESCE(SUM(attendees),0) as sum").
		Scan(&r2).Error
	return r2.Sum, err
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
	var rows []row
	err := r.db.WithContext(ctx).Model(&models.Event{}).
		Select("to_char(date::date, 'YYYY-MM') as month, COUNT(*) as count").
		Where("date ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'").
		Where("date::date >= (CURRENT_DATE - (? * INTERVAL '1 month'))", months-1).
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

func (r *analyticsRepository) IngestBatch(ctx context.Context, batch *models.AnalyticsBatch) error {
	return r.db.WithContext(ctx).Create(batch).Error
}
