package service

import (
	"context"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

// AdminAnalyticsResult is the strongly-typed response for GetAdminAnalytics.
type AdminAnalyticsResult struct {
	TotalEvents      int64            `json:"totalEvents"`
	UpcomingEvents   int64            `json:"upcomingEvents"`
	TotalAttendees   int64            `json:"totalAttendees"`
	EventsByCategory map[string]int64 `json:"eventsByCategory"`
	MonthlyStats     []any            `json:"monthlyStats"`
}

// AnalyticsService exposes aggregated analytics queries.
type AnalyticsService interface {
	GetAdminAnalytics(ctx context.Context) (*AdminAnalyticsResult, error)
	IngestBatch(ctx context.Context, batch *models.AnalyticsBatch) error
}

type analyticsService struct {
	repo repository.AnalyticsRepository
}

func NewAnalyticsService(repo repository.AnalyticsRepository) AnalyticsService {
	return &analyticsService{repo: repo}
}

func (s *analyticsService) GetAdminAnalytics(ctx context.Context) (*AdminAnalyticsResult, error) {
	total, err := s.repo.CountAllEvents(ctx)
	if err != nil {
		return nil, err
	}
	upcoming, err := s.repo.CountUpcomingEvents(ctx)
	if err != nil {
		return nil, err
	}
	attendees, err := s.repo.SumAttendees(ctx)
	if err != nil {
		return nil, err
	}
	byCategory, err := s.repo.CountEventsByCategory(ctx)
	if err != nil {
		return nil, err
	}
	return &AdminAnalyticsResult{
		TotalEvents:      total,
		UpcomingEvents:   upcoming,
		TotalAttendees:   attendees,
		EventsByCategory: byCategory,
		MonthlyStats:     []any{},
	}, nil
}

func (s *analyticsService) IngestBatch(ctx context.Context, batch *models.AnalyticsBatch) error {
	return s.repo.IngestBatch(ctx, batch)
}
