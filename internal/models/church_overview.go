package models

import "time"

// ChurchOverview is the single church-wide analytics payload consumed by the
// admin portal's Analytics and Reports pages. Every figure is measured from the
// database — there is no client-side stitching or estimation. Twelve-month
// series are always length 12, oldest first, zero-filled.
type ChurchOverview struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	Range       string         `json:"range"` // month | last30 | year
	Window      DecisionWindow `json:"window"`

	People     ChurchPeople      `json:"people"`
	Intake     ChurchIntake      `json:"intake"`
	Giving     ChurchGiving      `json:"giving"`
	Engagement ChurchEngagement  `json:"engagement"`
	Ministry   ChurchMinistry    `json:"ministry"`
	Content    ChurchContent     `json:"content"`
	Events     ChurchEventsBlock `json:"events"`

	Signals         DecisionSignalMetrics `json:"signals"`
	Recommendations []string              `json:"recommendations"`
}

// MonthPoint is one month in a zero-filled 12-month count series.
type MonthPoint struct {
	Month string `json:"month"` // "YYYY-MM"
	Value int64  `json:"value"`
}

// MoneyMonthPoint is one month of aggregated giving, in kobo.
type MoneyMonthPoint struct {
	Month      string `json:"month"` // "YYYY-MM"
	AmountKobo int64  `json:"amountKobo"`
}

// NamedCount is a labelled tally (by status, category, role, …).
type NamedCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// NamedMoney is a labelled kobo total (giving by category / channel).
type NamedMoney struct {
	Name       string `json:"name"`
	AmountKobo int64  `json:"amountKobo"`
	Count      int64  `json:"count"`
}

type ChurchPeople struct {
	MembersTotal      int64        `json:"membersTotal"`
	MembersActive     int64        `json:"membersActive"`
	MembersMonthly    []MonthPoint `json:"membersMonthly"`
	NewMembersInRange int64        `json:"newMembersInRange"`
	WorkforceTotal    int64        `json:"workforceTotal"`
	WorkforceServing  int64        `json:"workforceServing"`
	SubscribersTotal  int64        `json:"subscribersTotal"`
	SubscribersActive int64        `json:"subscribersActive"`
	SubscribersAdded  int64        `json:"subscribersAdded30d"`
	LeadershipTotal   int64        `json:"leadershipTotal"`
	LeadershipByRole  []NamedCount `json:"leadershipByRole"`
	LeadershipByState []NamedCount `json:"leadershipByStatus"`
}

type ChurchIntake struct {
	SubmissionsTotal    int64        `json:"submissionsTotal"`
	SubmissionsCurrent  int64        `json:"submissionsCurrent"`
	SubmissionsPrevious int64        `json:"submissionsPrevious"`
	PerForm             []FormTally  `json:"perForm"`
	NewMemberMonthly    []MonthPoint `json:"newMemberMonthly"`
	WorkflowByStage     []NamedCount `json:"workflowByStage"`
	WorkflowStalled     int64        `json:"workflowStalled"`
}

// FormTally is one form's all-time submission count.
type FormTally struct {
	FormID string `json:"formId"`
	Title  string `json:"title"`
	Count  int64  `json:"count"`
}

type ChurchGiving struct {
	ThisMonthKobo int64             `json:"thisMonthKobo"`
	LastMonthKobo int64             `json:"lastMonthKobo"`
	YtdKobo       int64             `json:"ytdKobo"`
	Monthly       []MoneyMonthPoint `json:"monthly"`
	ByCategory    []NamedMoney      `json:"byCategory"`
	ByChannel     []NamedMoney      `json:"byChannel"`
	AvgGiftKobo   int64             `json:"avgGiftKobo"`
	SuccessCount  int64             `json:"successCount"`
	FailedCount   int64             `json:"failedCount"`
	HasData       bool              `json:"hasData"`
}

type ChurchEngagement struct {
	AttendanceCurrent       int64         `json:"attendanceCurrent"`
	AttendancePrevious      int64         `json:"attendancePrevious"`
	AttendanceMonthly       []MonthPoint  `json:"attendanceMonthly"`
	AttendanceByServiceType []NamedCount  `json:"attendanceByServiceType"`
	Backlog                 ChurchBacklog `json:"backlog"`
}

type ChurchBacklog struct {
	PrayerOpen           int64        `json:"prayerOpen"`
	PrayerByStatus       []NamedCount `json:"prayerByStatus"`
	PrayerOldestOpenDays int64        `json:"prayerOldestOpenDays"`
	ContactTotal         int64        `json:"contactTotal"`
	Contact30d           int64        `json:"contact30d"`
	VisitsUpcoming       int64        `json:"visitsUpcoming"`
	VisitsByStatus       []NamedCount `json:"visitsByStatus"`
	PastoralByType       []NamedCount `json:"pastoralByType"`
	PastoralTotal        int64        `json:"pastoralTotal"`
	ApprovalsPending     int64        `json:"approvalsPending"`
}

type ChurchMinistry struct {
	CellGroupsCount      int64        `json:"cellGroupsCount"`
	CellGroupMembers     int64        `json:"cellGroupMembers"`
	CellGroupAvgSize     float64      `json:"cellGroupAvgSize"`
	CellGroupMeetings30d int64        `json:"cellGroupMeetings30d"`
	MinistriesCount      int64        `json:"ministriesCount"`
	MinistriesUnstaffed  int64        `json:"ministriesUnstaffed"`
	MembersByMinistry    []NamedCount `json:"membersByMinistry"`
}

type ChurchContent struct {
	TestimonialsApproved int64        `json:"testimonialsApproved"`
	TestimonialsPending  int64        `json:"testimonialsPending"`
	Testimonials30d      int64        `json:"testimonials30d"`
	StoreRevenue         float64      `json:"storeRevenue"`
	StoreOrdersByStatus  []NamedCount `json:"storeOrdersByStatus"`
	StorePaymentPending  int64        `json:"storePaymentPending"`
}

type ChurchEventsBlock struct {
	Total      int64        `json:"total"`
	Upcoming   int64        `json:"upcoming"`
	ByCategory []NamedCount `json:"byCategory"`
	Monthly    []MonthPoint `json:"monthly"`
	HasData    bool         `json:"hasData"`
}
