package service

import (
	"context"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

// AdminAnalyticsResult is the strongly-typed response for GetAdminAnalytics.
type AdminAnalyticsResult struct {
	TotalEvents      int64                          `json:"totalEvents"`
	UpcomingEvents   int64                          `json:"upcomingEvents"`
	TotalAttendees   int64                          `json:"totalAttendees"`
	EventsByCategory map[string]int64               `json:"eventsByCategory"`
	MonthlyStats     []repository.MonthlyEventStat  `json:"monthlyStats"`
	Operations       *repository.OperationalSummary `json:"operations"`
	GeneratedAt      time.Time                      `json:"generatedAt"`
}

// AnalyticsService exposes aggregated analytics queries.
type AnalyticsService interface {
	GetAdminAnalytics(ctx context.Context) (*AdminAnalyticsResult, error)
	IngestBatch(ctx context.Context, batch *models.AnalyticsBatch, events []models.AnalyticsEvent) error
}

type analyticsService struct {
	repo repository.AnalyticsRepository
}

func NewAnalyticsService(repo repository.AnalyticsRepository) AnalyticsService {
	return &analyticsService{repo: repo}
}

func (s *analyticsService) GetAdminAnalytics(ctx context.Context) (*AdminAnalyticsResult, error) {
	events, err := s.repo.EventSummary(ctx)
	if err != nil {
		return nil, err
	}
	byCategory, err := s.repo.CountEventsByCategory(ctx)
	if err != nil {
		return nil, err
	}
	monthly, err := s.repo.EventsByMonth(ctx, 12)
	if err != nil {
		return nil, err
	}
	operations, err := s.repo.OperationalSummary(ctx)
	if err != nil {
		return nil, err
	}
	return &AdminAnalyticsResult{
		TotalEvents:      events.Total,
		UpcomingEvents:   events.Upcoming,
		TotalAttendees:   operations.TotalAttendance,
		EventsByCategory: byCategory,
		MonthlyStats:     monthly,
		Operations:       operations,
		GeneratedAt:      time.Now().UTC(),
	}, nil
}

func (s *analyticsService) IngestBatch(ctx context.Context, batch *models.AnalyticsBatch, events []models.AnalyticsEvent) error {
	return s.repo.IngestBatch(ctx, batch, events)
}
