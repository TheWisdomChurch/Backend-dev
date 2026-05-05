package models

import (
	"time"

	"gorm.io/datatypes"
)

type GrowthBucket struct {
	Period string `json:"period"`
	Count  int64  `json:"count"`
}

type NewMemberFormSummary struct {
	FormID           string     `json:"formId"`
	FormTitle        string     `json:"formTitle"`
	Slug             *string    `json:"slug,omitempty"`
	IsPublished      bool       `json:"isPublished"`
	SubmissionCount  int64      `json:"submissionCount"`
	LastSubmissionAt *time.Time `json:"lastSubmissionAt,omitempty"`
}

type NewMemberSubmission struct {
	ID               string         `json:"id"`
	FormID           string         `json:"formId"`
	FormTitle        string         `json:"formTitle"`
	Name             *string        `json:"name,omitempty"`
	Email            *string        `json:"email,omitempty"`
	ContactNumber    *string        `json:"contactNumber,omitempty"`
	ContactAddress   *string        `json:"contactAddress,omitempty"`
	RegistrationCode *string        `json:"registrationCode,omitempty"`
	Values           datatypes.JSON `json:"values"`
	CreatedAt        time.Time      `json:"createdAt"`
}

type NewMemberDashboardResponse struct {
	TotalSubmissions int64                  `json:"totalSubmissions"`
	ThisWeek         int64                  `json:"thisWeek"`
	ThisMonth        int64                  `json:"thisMonth"`
	ThisQuarter      int64                  `json:"thisQuarter"`
	ThisYear         int64                  `json:"thisYear"`
	Forms            []NewMemberFormSummary `json:"forms"`
	Recent           []NewMemberSubmission  `json:"recent"`
	WeeklyGrowth     []GrowthBucket         `json:"weeklyGrowth"`
	MonthlyGrowth    []GrowthBucket         `json:"monthlyGrowth"`
	QuarterlyGrowth  []GrowthBucket         `json:"quarterlyGrowth"`
	YearlyGrowth     []GrowthBucket         `json:"yearlyGrowth"`
}

type MemberStatsResponse struct {
	Total         int64          `json:"total"`
	Active        int64          `json:"active"`
	Inactive      int64          `json:"inactive"`
	MonthlyGrowth []GrowthBucket `json:"monthlyGrowth"`
	YearlyGrowth  []GrowthBucket `json:"yearlyGrowth"`
}
