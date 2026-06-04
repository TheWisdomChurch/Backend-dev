package repository

import (
	"context"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

// AnalyticsRepository handles raw analytics data persistence and aggregation.
type AnalyticsRepository interface {
	CountAllEvents(ctx context.Context) (int64, error)
	CountUpcomingEvents(ctx context.Context) (int64, error)
	SumAttendees(ctx context.Context) (int64, error)
	CountEventsByCategory(ctx context.Context) (map[string]int64, error)
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

func (r *analyticsRepository) IngestBatch(ctx context.Context, batch *models.AnalyticsBatch) error {
	return r.db.WithContext(ctx).Create(batch).Error
}
