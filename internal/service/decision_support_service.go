package service

import (
	"context"
	"log/slog"
	"math"
	"time"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/metrics"
	"wisdomHouse-backend/internal/models"
)

type DecisionSupportService interface {
	GetInsights(ctx context.Context) (*models.DecisionInsights, error)
}

type decisionSupportService struct {
	db    *database.Database
	cache *cache.RedisClient
}

func NewDecisionSupportService(db *database.Database, redisCache *cache.RedisClient) DecisionSupportService {
	return &decisionSupportService{
		db:    db,
		cache: redisCache,
	}
}

func (s *decisionSupportService) GetInsights(ctx context.Context) (*models.DecisionInsights, error) {
	cacheKey := "analytics:decision_insights:v1"
	if s.cache != nil {
		var cached models.DecisionInsights
		if err := s.cache.GetJSON(ctx, cacheKey, &cached); err == nil {
			metrics.RecordAnalyticsCache("hit")
			return &cached, nil
		}
		metrics.RecordAnalyticsCache("miss")
	}

	now := time.Now().UTC()
	currentStart := now.AddDate(0, 0, -30)
	previousStart := currentStart.AddDate(0, 0, -30)

	type insightCounts struct {
		TotalEvents, UpcomingEvents, TotalMembers, ActiveMembers int64
		WorkforceTotal, WorkforceServing                         int64
		SubmissionsCurrent30d, SubmissionsPrevious30d            int64
	}
	var counts insightCounts
	// members and workforce_members are hard-delete tables (no deleted_at
	// column) — only form_submissions is soft-deletable.
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM events) AS total_events,
			(SELECT COUNT(*) FROM events WHERE event_date >= CURRENT_DATE) AS upcoming_events,
			(SELECT COUNT(*) FROM members) AS total_members,
			(SELECT COUNT(*) FROM members WHERE is_active = TRUE) AS active_members,
			(SELECT COUNT(*) FROM workforce_members) AS workforce_total,
			(SELECT COUNT(*) FROM workforce_members WHERE status = ?) AS workforce_serving,
			(SELECT COUNT(*) FROM form_submissions WHERE deleted_at IS NULL AND created_at >= ? AND created_at < ?) AS submissions_current30d,
			(SELECT COUNT(*) FROM form_submissions WHERE deleted_at IS NULL AND created_at >= ? AND created_at < ?) AS submissions_previous30d
	`, models.WorkforceStatusServing, currentStart, now, previousStart, currentStart).Scan(&counts).Error; err != nil {
		return nil, err
	}
	totalEvents, upcomingEvents := counts.TotalEvents, counts.UpcomingEvents
	totalMembers, activeMembers := counts.TotalMembers, counts.ActiveMembers
	workforceTotal, workforceServing := counts.WorkforceTotal, counts.WorkforceServing
	submissionsCurrent30d, submissionsPrevious30d := counts.SubmissionsCurrent30d, counts.SubmissionsPrevious30d

	var submissionsDeltaPercent float64
	switch {
	case submissionsPrevious30d == 0 && submissionsCurrent30d > 0:
		submissionsDeltaPercent = 100
	case submissionsPrevious30d == 0:
		submissionsDeltaPercent = 0
	default:
		submissionsDeltaPercent = ((float64(submissionsCurrent30d-submissionsPrevious30d) / float64(submissionsPrevious30d)) * 100)
	}

	memberActivationRate := ratio(activeMembers, totalMembers)
	volunteerCoverageRate := ratio(workforceServing, workforceTotal)
	upcomingEventLoadRate := ratio(upcomingEvents, max64(totalEvents, 1))
	submissionMomentum := normalize(submissionsDeltaPercent, -100, 100)

	decisionReadinessScore := clamp(
		(0.35*submissionMomentum+
			0.25*memberActivationRate+
			0.25*volunteerCoverageRate+
			0.15*(1-upcomingEventLoadRate))*100,
		0,
		100,
	)

	recommendations := make([]string, 0, 4)
	if submissionsDeltaPercent < 0 {
		recommendations = append(recommendations, "Submission volume is dropping. Trigger targeted follow-up campaigns for inactive segments within 7 days.")
	}
	if volunteerCoverageRate < 0.45 {
		recommendations = append(recommendations, "Volunteer coverage is low. Prioritize workforce recruitment and automate onboarding reminders.")
	}
	if memberActivationRate < 0.7 {
		recommendations = append(recommendations, "Active member ratio is below target. Launch retention journeys for members with no recent engagement.")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Core indicators are healthy. Maintain cadence and validate forecasts weekly.")
	}

	out := &models.DecisionInsights{
		GeneratedAt: now,
		Window: models.DecisionWindow{
			CurrentStart:  currentStart,
			CurrentEnd:    now,
			PreviousStart: previousStart,
			PreviousEnd:   currentStart,
		},
		Core: models.DecisionCoreMetrics{
			TotalEvents:            totalEvents,
			UpcomingEvents:         upcomingEvents,
			TotalMembers:           totalMembers,
			ActiveMembers:          activeMembers,
			TotalWorkforce:         workforceTotal,
			ServingWorkforce:       workforceServing,
			SubmissionsCurrent30d:  submissionsCurrent30d,
			SubmissionsPrevious30d: submissionsPrevious30d,
		},
		Signals: models.DecisionSignalMetrics{
			MemberActivationRate:   memberActivationRate,
			VolunteerCoverageRate:  volunteerCoverageRate,
			UpcomingEventLoadRate:  upcomingEventLoadRate,
			SubmissionDeltaPercent: round(submissionsDeltaPercent, 2),
			DecisionReadinessScore: round(decisionReadinessScore, 2),
		},
		Recommendations: recommendations,
	}

	if s.cache != nil {
		if err := s.cache.SetJSON(ctx, cacheKey, out, 5*time.Minute); err != nil {
			metrics.RecordAnalyticsCache("write_error")
			slog.Warn("decision insights cache write failed", "error", err)
		} else {
			metrics.RecordAnalyticsCache("write_success")
		}
	}

	return out, nil
}

func ratio(a, b int64) float64 {
	if b <= 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func normalize(v, minV, maxV float64) float64 {
	if maxV <= minV {
		return 0
	}
	n := (v - minV) / (maxV - minV)
	return clamp(n, 0, 1)
}

func round(v float64, dp int) float64 {
	p := math.Pow(10, float64(dp))
	return math.Round(v*p) / p
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
