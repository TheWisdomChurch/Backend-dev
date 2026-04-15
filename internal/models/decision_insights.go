package models

import "time"

type DecisionInsights struct {
	GeneratedAt     time.Time             `json:"generatedAt"`
	Window          DecisionWindow        `json:"window"`
	Core            DecisionCoreMetrics   `json:"core"`
	Signals         DecisionSignalMetrics `json:"signals"`
	Recommendations []string              `json:"recommendations"`
}

type DecisionWindow struct {
	CurrentStart  time.Time `json:"currentStart"`
	CurrentEnd    time.Time `json:"currentEnd"`
	PreviousStart time.Time `json:"previousStart"`
	PreviousEnd   time.Time `json:"previousEnd"`
}

type DecisionCoreMetrics struct {
	TotalEvents            int64 `json:"totalEvents"`
	UpcomingEvents         int64 `json:"upcomingEvents"`
	TotalMembers           int64 `json:"totalMembers"`
	ActiveMembers          int64 `json:"activeMembers"`
	TotalWorkforce         int64 `json:"totalWorkforce"`
	ServingWorkforce       int64 `json:"servingWorkforce"`
	SubmissionsCurrent30d  int64 `json:"submissionsCurrent30d"`
	SubmissionsPrevious30d int64 `json:"submissionsPrevious30d"`
}

type DecisionSignalMetrics struct {
	MemberActivationRate   float64 `json:"memberActivationRate"`
	VolunteerCoverageRate  float64 `json:"volunteerCoverageRate"`
	UpcomingEventLoadRate  float64 `json:"upcomingEventLoadRate"`
	SubmissionDeltaPercent float64 `json:"submissionDeltaPercent"`
	DecisionReadinessScore float64 `json:"decisionReadinessScore"`
}
