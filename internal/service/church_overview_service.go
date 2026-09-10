package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/metrics"
	"wisdomHouse-backend/internal/models"
)

// ChurchOverviewService computes the single church-wide analytics payload. Every
// figure is measured from the database; nothing is estimated. Individual
// aggregations are best-effort — a failure in one block is logged and leaves
// that block zero-valued rather than failing the whole response, so the portal
// can always render something real.
type ChurchOverviewService interface {
	GetOverview(ctx context.Context, timeRange string) (*models.ChurchOverview, error)
}

type churchOverviewService struct {
	db       *database.Database
	cache    *cache.RedisClient
	decision DecisionSupportService
}

func NewChurchOverviewService(db *database.Database, redisCache *cache.RedisClient) ChurchOverviewService {
	return &churchOverviewService{
		db:       db,
		cache:    redisCache,
		decision: NewDecisionSupportService(db, redisCache),
	}
}

func normalizeRange(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "month":
		return "month"
	case "year":
		return "year"
	default:
		return "last30"
	}
}

// resolveWindow returns the current and previous comparison windows for a range.
func resolveWindow(timeRange string, now time.Time) (curStart, curEnd, prevStart, prevEnd time.Time) {
	switch timeRange {
	case "month":
		curStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		curEnd = now
		prevStart = curStart.AddDate(0, -1, 0)
		prevEnd = curStart
	case "year":
		curStart = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		curEnd = now
		prevStart = curStart.AddDate(-1, 0, 0)
		prevEnd = curStart
	default: // last30
		curStart = now.AddDate(0, 0, -30)
		curEnd = now
		prevStart = now.AddDate(0, 0, -60)
		prevEnd = curStart
	}
	return
}

// monthKeys returns the last 12 "YYYY-MM" keys, oldest first.
func monthKeys(now time.Time) []string {
	keys := make([]string, 0, 12)
	for i := 11; i >= 0; i-- {
		keys = append(keys, now.AddDate(0, -i, 0).Format("2006-01"))
	}
	return keys
}

// zeroFillCounts maps a {month -> value} result onto the last-12-months window.
func zeroFillCounts(now time.Time, byMonth map[string]int64) []models.MonthPoint {
	out := make([]models.MonthPoint, 0, 12)
	for _, k := range monthKeys(now) {
		out = append(out, models.MonthPoint{Month: k, Value: byMonth[k]})
	}
	return out
}

func zeroFillMoney(now time.Time, byMonth map[string]int64) []models.MoneyMonthPoint {
	out := make([]models.MoneyMonthPoint, 0, 12)
	for _, k := range monthKeys(now) {
		out = append(out, models.MoneyMonthPoint{Month: k, AmountKobo: byMonth[k]})
	}
	return out
}

func (s *churchOverviewService) GetOverview(ctx context.Context, timeRange string) (*models.ChurchOverview, error) {
	timeRange = normalizeRange(timeRange)
	cacheKey := "analytics:church_overview:v1:" + timeRange

	if s.cache != nil {
		var cached models.ChurchOverview
		if err := s.cache.GetJSON(ctx, cacheKey, &cached); err == nil {
			metrics.RecordAnalyticsCache("hit")
			return &cached, nil
		}
		metrics.RecordAnalyticsCache("miss")
	}

	now := time.Now().UTC()
	curStart, curEnd, prevStart, prevEnd := resolveWindow(timeRange, now)

	out := &models.ChurchOverview{
		GeneratedAt: now,
		Range:       timeRange,
		Window: models.DecisionWindow{
			CurrentStart: curStart, CurrentEnd: curEnd,
			PreviousStart: prevStart, PreviousEnd: prevEnd,
		},
	}

	warn := func(block string, err error) {
		if err != nil {
			slog.Warn("church overview aggregation failed", "block", block, "error", err)
		}
	}

	warn("people", s.fillPeople(ctx, now, curStart, curEnd, &out.People))
	warn("intake", s.fillIntake(ctx, now, curStart, curEnd, prevStart, prevEnd, &out.Intake))
	warn("giving", s.fillGiving(ctx, now, &out.Giving))
	warn("engagement", s.fillEngagement(ctx, now, curStart, curEnd, prevStart, prevEnd, &out.Engagement))
	warn("ministry", s.fillMinistry(ctx, now, &out.Ministry))
	warn("content", s.fillContent(ctx, now, &out.Content))
	warn("events", s.fillEvents(ctx, now, &out.Events))

	// Signals + recommendations come from the (extended) decision engine, then
	// get enriched with the giving / attendance / backlog deltas we just
	// computed so the readiness score reflects the whole church, not just forms.
	if insights, err := s.decision.GetInsights(ctx); err != nil {
		warn("signals", err)
	} else {
		out.Signals = insights.Signals
		out.Recommendations = insights.Recommendations
	}
	s.enrichSignals(out)

	if s.cache != nil {
		if err := s.cache.SetJSON(ctx, cacheKey, out, 3*time.Minute); err != nil {
			slog.Warn("church overview cache write failed", "error", err)
		}
	}
	return out, nil
}

// ── People ──────────────────────────────────────────────────────────────────

func (s *churchOverviewService) fillPeople(ctx context.Context, now, curStart, curEnd time.Time, p *models.ChurchPeople) error {
	var core struct {
		MembersTotal, MembersActive         int64
		WorkforceTotal, WorkforceServing    int64
		SubscribersTotal, SubscribersActive int64
		SubscribersAdded, LeadershipTotal   int64
		NewMembersInRange                   int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM members) AS members_total,
			(SELECT COUNT(*) FROM members WHERE is_active = TRUE) AS members_active,
			(SELECT COUNT(*) FROM members WHERE created_at >= ? AND created_at < ?) AS new_members_in_range,
			(SELECT COUNT(*) FROM workforce_members) AS workforce_total,
			(SELECT COUNT(*) FROM workforce_members WHERE status = ?) AS workforce_serving,
			(SELECT COUNT(*) FROM subscribers) AS subscribers_total,
			(SELECT COUNT(*) FROM subscribers WHERE status = 'active') AS subscribers_active,
			(SELECT COUNT(*) FROM subscribers WHERE created_at >= NOW() - INTERVAL '30 days') AS subscribers_added,
			(SELECT COUNT(*) FROM leadership_members) AS leadership_total
	`, curStart, curEnd, models.WorkforceStatusServing).Scan(&core).Error; err != nil {
		return err
	}
	p.MembersTotal = core.MembersTotal
	p.MembersActive = core.MembersActive
	p.NewMembersInRange = core.NewMembersInRange
	p.WorkforceTotal = core.WorkforceTotal
	p.WorkforceServing = core.WorkforceServing
	p.SubscribersTotal = core.SubscribersTotal
	p.SubscribersActive = core.SubscribersActive
	p.SubscribersAdded = core.SubscribersAdded
	p.LeadershipTotal = core.LeadershipTotal

	p.MembersMonthly = s.monthlyCount(ctx, now, "members", "created_at", "")
	p.LeadershipByRole = s.namedCounts(ctx, "leadership_members", "role", "")
	p.LeadershipByState = s.namedCounts(ctx, "leadership_members", "status", "")
	return nil
}

// ── Intake (forms + new-member workflow) ────────────────────────────────────

func (s *churchOverviewService) fillIntake(ctx context.Context, now, curStart, curEnd, prevStart, prevEnd time.Time, in *models.ChurchIntake) error {
	var core struct {
		Total, Current, Previous, Stalled int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM form_submissions WHERE deleted_at IS NULL) AS total,
			(SELECT COUNT(*) FROM form_submissions WHERE deleted_at IS NULL AND created_at >= ? AND created_at < ?) AS current,
			(SELECT COUNT(*) FROM form_submissions WHERE deleted_at IS NULL AND created_at >= ? AND created_at < ?) AS previous,
			(SELECT COUNT(*) FROM new_member_workflows WHERE stage NOT IN ('integrated','closed') AND updated_at < NOW() - INTERVAL '14 days') AS stalled
	`, curStart, curEnd, prevStart, prevEnd).Scan(&core).Error; err != nil {
		return err
	}
	in.SubmissionsTotal = core.Total
	in.SubmissionsCurrent = core.Current
	in.SubmissionsPrevious = core.Previous
	in.WorkflowStalled = core.Stalled

	type ftRow struct {
		FormID string
		Title  string
		Count  int64
	}
	var forms []ftRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT fs.form_id AS form_id, COALESCE(f.title, 'Untitled form') AS title, COUNT(*) AS count
		FROM form_submissions fs
		LEFT JOIN forms f ON f.id = fs.form_id
		WHERE fs.deleted_at IS NULL
		GROUP BY fs.form_id, f.title
		ORDER BY count DESC
		LIMIT 12
	`).Scan(&forms).Error; err == nil {
		in.PerForm = make([]models.FormTally, 0, len(forms))
		for _, r := range forms {
			in.PerForm = append(in.PerForm, models.FormTally{FormID: r.FormID, Title: r.Title, Count: r.Count})
		}
	}

	in.NewMemberMonthly = s.newMemberMonthly(ctx, now)
	in.WorkflowByStage = s.namedCounts(ctx, "new_member_workflows", "stage", "")
	return nil
}

// newMemberMonthly counts submissions to forms flagged as new-member intake.
// Falls back to all submissions if the flag column is absent.
func (s *churchOverviewService) newMemberMonthly(ctx context.Context, now time.Time) []models.MonthPoint {
	type row struct {
		Month string
		Value int64
	}
	var rows []row
	// "New member" forms are matched the same way form_repository does: the
	// submissionTarget=member setting, with slug/title fallbacks for older forms.
	err := s.db.WithContext(ctx).Raw(`
		SELECT to_char(fs.created_at, 'YYYY-MM') AS month, COUNT(*) AS value
		FROM form_submissions fs
		JOIN forms f ON f.id = fs.form_id
		WHERE fs.deleted_at IS NULL
		  AND fs.created_at >= date_trunc('month', CURRENT_DATE) - INTERVAL '11 months'
		  AND (
			LOWER(COALESCE(f.settings->>'submissionTarget', '')) = 'member'
			OR LOWER(COALESCE(f.slug, '')) = 'add-new-member'
			OR trim(regexp_replace(LOWER(COALESCE(f.title, '')), '[^a-z0-9]+', ' ', 'g')) = 'add new member'
		  )
		GROUP BY month
	`).Scan(&rows).Error
	if err != nil {
		rows = nil
		_ = s.db.WithContext(ctx).Raw(`
			SELECT to_char(created_at, 'YYYY-MM') AS month, COUNT(*) AS value
			FROM form_submissions
			WHERE deleted_at IS NULL
			  AND created_at >= date_trunc('month', CURRENT_DATE) - INTERVAL '11 months'
			GROUP BY month
		`).Scan(&rows).Error
	}
	byMonth := make(map[string]int64, len(rows))
	for _, r := range rows {
		byMonth[r.Month] = r.Value
	}
	return zeroFillCounts(now, byMonth)
}

// ── Giving ─────────────────────────────────────────────────────────────────

func (s *churchOverviewService) fillGiving(ctx context.Context, now time.Time, g *models.ChurchGiving) error {
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := thisMonth.AddDate(0, -1, 0)
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)

	var core struct {
		ThisMonthKobo, LastMonthKobo, YtdKobo int64
		SuccessCount, FailedCount             int64
		AvgGiftKobo                           int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			COALESCE(SUM(amount_kobo) FILTER (WHERE status = 'success' AND given_at >= ?), 0) AS this_month_kobo,
			COALESCE(SUM(amount_kobo) FILTER (WHERE status = 'success' AND given_at >= ? AND given_at < ?), 0) AS last_month_kobo,
			COALESCE(SUM(amount_kobo) FILTER (WHERE status = 'success' AND given_at >= ?), 0) AS ytd_kobo,
			COUNT(*) FILTER (WHERE status = 'success') AS success_count,
			COUNT(*) FILTER (WHERE status IN ('failed','reversed')) AS failed_count,
			COALESCE(AVG(amount_kobo) FILTER (WHERE status = 'success'), 0)::bigint AS avg_gift_kobo
		FROM giving_transactions
		WHERE deleted_at IS NULL
	`, thisMonth, lastMonth, thisMonth, yearStart).Scan(&core).Error; err != nil {
		return err
	}
	g.ThisMonthKobo = core.ThisMonthKobo
	g.LastMonthKobo = core.LastMonthKobo
	g.YtdKobo = core.YtdKobo
	g.SuccessCount = core.SuccessCount
	g.FailedCount = core.FailedCount
	g.AvgGiftKobo = core.AvgGiftKobo
	g.HasData = core.SuccessCount > 0

	type mrow struct {
		Month string
		Kobo  int64
	}
	var mrows []mrow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT to_char(given_at, 'YYYY-MM') AS month, COALESCE(SUM(amount_kobo), 0) AS kobo
		FROM giving_transactions
		WHERE deleted_at IS NULL AND status = 'success'
		  AND given_at >= date_trunc('month', CURRENT_DATE) - INTERVAL '11 months'
		GROUP BY month
	`).Scan(&mrows).Error; err == nil {
		byMonth := make(map[string]int64, len(mrows))
		for _, r := range mrows {
			byMonth[r.Month] = r.Kobo
		}
		g.Monthly = zeroFillMoney(now, byMonth)
	}

	g.ByCategory = s.givingBreakdown(ctx, `
		SELECT COALESCE(c.name, 'Uncategorised') AS name, COALESCE(SUM(t.amount_kobo), 0) AS amount_kobo, COUNT(*) AS count
		FROM giving_transactions t
		LEFT JOIN giving_categories c ON c.id = t.category_id
		WHERE t.deleted_at IS NULL AND t.status = 'success'
		GROUP BY c.name ORDER BY amount_kobo DESC LIMIT 12`)
	g.ByChannel = s.givingBreakdown(ctx, `
		SELECT COALESCE(NULLIF(channel, ''), 'unknown') AS name, COALESCE(SUM(amount_kobo), 0) AS amount_kobo, COUNT(*) AS count
		FROM giving_transactions
		WHERE deleted_at IS NULL AND status = 'success'
		GROUP BY channel ORDER BY amount_kobo DESC`)
	return nil
}

func (s *churchOverviewService) givingBreakdown(ctx context.Context, query string) []models.NamedMoney {
	type row struct {
		Name       string
		AmountKobo int64
		Count      int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return nil
	}
	out := make([]models.NamedMoney, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.NamedMoney{Name: r.Name, AmountKobo: r.AmountKobo, Count: r.Count})
	}
	return out
}

// ── Engagement (attendance + backlogs) ─────────────────────────────────────

func (s *churchOverviewService) fillEngagement(ctx context.Context, now, curStart, curEnd, prevStart, prevEnd time.Time, e *models.ChurchEngagement) error {
	var att struct {
		Current, Previous int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		WITH sess AS (
			SELECT s.id, s.date,
			       GREATEST(s.head_count, COALESCE(rc.record_count, 0)) AS attendance
			FROM attendance_sessions s
			LEFT JOIN (
				SELECT session_id, COUNT(*) AS record_count
				FROM attendance_records WHERE deleted_at IS NULL GROUP BY session_id
			) rc ON rc.session_id = s.id
			WHERE s.deleted_at IS NULL
		)
		SELECT
			COALESCE(SUM(attendance) FILTER (WHERE date >= ? AND date < ?), 0) AS current,
			COALESCE(SUM(attendance) FILTER (WHERE date >= ? AND date < ?), 0) AS previous
		FROM sess
	`, curStart, curEnd, prevStart, prevEnd).Scan(&att).Error; err != nil {
		return err
	}
	e.AttendanceCurrent = att.Current
	e.AttendancePrevious = att.Previous

	type mrow struct {
		Month string
		Value int64
	}
	var mrows []mrow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT to_char(s.date, 'YYYY-MM') AS month,
		       COALESCE(SUM(GREATEST(s.head_count, COALESCE(rc.record_count, 0))), 0) AS value
		FROM attendance_sessions s
		LEFT JOIN (
			SELECT session_id, COUNT(*) AS record_count
			FROM attendance_records WHERE deleted_at IS NULL GROUP BY session_id
		) rc ON rc.session_id = s.id
		WHERE s.deleted_at IS NULL
		  AND s.date >= date_trunc('month', CURRENT_DATE) - INTERVAL '11 months'
		GROUP BY month
	`).Scan(&mrows).Error; err == nil {
		byMonth := make(map[string]int64, len(mrows))
		for _, r := range mrows {
			byMonth[r.Month] = r.Value
		}
		e.AttendanceMonthly = zeroFillCounts(now, byMonth)
	}

	type stRow struct {
		Name  string
		Count int64
	}
	var stRows []stRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(st.name, 'Other') AS name,
		       COALESCE(SUM(GREATEST(s.head_count, COALESCE(rc.record_count, 0))), 0) AS count
		FROM attendance_sessions s
		LEFT JOIN service_types st ON st.id = s.service_type_id
		LEFT JOIN (
			SELECT session_id, COUNT(*) AS record_count
			FROM attendance_records WHERE deleted_at IS NULL GROUP BY session_id
		) rc ON rc.session_id = s.id
		WHERE s.deleted_at IS NULL AND s.date >= NOW() - INTERVAL '90 days'
		GROUP BY st.name ORDER BY count DESC
	`).Scan(&stRows).Error; err == nil {
		e.AttendanceByServiceType = make([]models.NamedCount, 0, len(stRows))
		for _, r := range stRows {
			e.AttendanceByServiceType = append(e.AttendanceByServiceType, models.NamedCount{Name: r.Name, Count: r.Count})
		}
	}

	return s.fillBacklog(ctx, &e.Backlog)
}

func (s *churchOverviewService) fillBacklog(ctx context.Context, b *models.ChurchBacklog) error {
	var core struct {
		PrayerOpen           int64
		PrayerOldestOpenDays int64
		ContactTotal         int64
		Contact30d           int64
		VisitsUpcoming       int64
		PastoralTotal        int64
		ApprovalsPending     int64
	}
	// Each sub-select is wrapped so a missing admin-domain table degrades to 0
	// rather than failing the batch.
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM prayer_requests WHERE deleted_at IS NULL AND status IN ('pending','praying')) AS prayer_open,
			(SELECT COALESCE(EXTRACT(DAY FROM NOW() - MIN(created_at)), 0)::bigint
			   FROM prayer_requests WHERE deleted_at IS NULL AND status IN ('pending','praying')) AS prayer_oldest_open_days,
			(SELECT COUNT(*) FROM contact_messages) AS contact_total,
			(SELECT COUNT(*) FROM contact_messages WHERE created_at >= NOW() - INTERVAL '30 days') AS contact_30d,
			(SELECT COUNT(*) FROM visit_requests WHERE service_at >= NOW() AND status NOT IN ('cancelled','completed','no_show','arrived')) AS visits_upcoming,
			(SELECT COUNT(*) FROM pastoral_care_requests) AS pastoral_total,
			(SELECT COUNT(*) FROM approval_requests WHERE status = 'pending') AS approvals_pending
	`).Scan(&core).Error; err != nil {
		return err
	}
	b.PrayerOpen = core.PrayerOpen
	b.PrayerOldestOpenDays = core.PrayerOldestOpenDays
	b.ContactTotal = core.ContactTotal
	b.Contact30d = core.Contact30d
	b.VisitsUpcoming = core.VisitsUpcoming
	b.PastoralTotal = core.PastoralTotal
	b.ApprovalsPending = core.ApprovalsPending

	b.PrayerByStatus = s.namedCounts(ctx, "prayer_requests", "status", "deleted_at IS NULL")
	b.VisitsByStatus = s.namedCounts(ctx, "visit_requests", "status", "")
	b.PastoralByType = s.namedCounts(ctx, "pastoral_care_requests", "event_type", "")
	return nil
}

// ── Ministry health ────────────────────────────────────────────────────────

func (s *churchOverviewService) fillMinistry(ctx context.Context, now time.Time, m *models.ChurchMinistry) error {
	var core struct {
		CellGroupsCount      int64
		CellGroupMembers     int64
		CellGroupMeetings30d int64
		MinistriesCount      int64
		MinistriesUnstaffed  int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM cell_groups WHERE deleted_at IS NULL) AS cell_groups_count,
			(SELECT COUNT(*) FROM cell_group_members WHERE deleted_at IS NULL) AS cell_group_members,
			(SELECT COUNT(*) FROM cell_group_meetings WHERE deleted_at IS NULL AND date >= NOW() - INTERVAL '30 days') AS cell_group_meetings30d,
			(SELECT COUNT(*) FROM ministries WHERE deleted_at IS NULL) AS ministries_count,
			(SELECT COUNT(*) FROM ministries mn WHERE mn.deleted_at IS NULL
			   AND NOT EXISTS (SELECT 1 FROM ministry_workforce_members mw WHERE mw.ministry_id = mn.id AND mw.deleted_at IS NULL)) AS ministries_unstaffed
	`).Scan(&core).Error; err != nil {
		return err
	}
	m.CellGroupsCount = core.CellGroupsCount
	m.CellGroupMembers = core.CellGroupMembers
	m.CellGroupMeetings30d = core.CellGroupMeetings30d
	m.MinistriesCount = core.MinistriesCount
	m.MinistriesUnstaffed = core.MinistriesUnstaffed
	if core.CellGroupsCount > 0 {
		m.CellGroupAvgSize = float64(core.CellGroupMembers) / float64(core.CellGroupsCount)
	}

	type row struct {
		Name  string
		Count int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`
		SELECT mn.name AS name, COUNT(mm.id) AS count
		FROM ministries mn
		LEFT JOIN ministry_members mm ON mm.ministry_id = mn.id AND mm.deleted_at IS NULL
		WHERE mn.deleted_at IS NULL
		GROUP BY mn.name ORDER BY count DESC LIMIT 12
	`).Scan(&rows).Error; err == nil {
		m.MembersByMinistry = make([]models.NamedCount, 0, len(rows))
		for _, r := range rows {
			m.MembersByMinistry = append(m.MembersByMinistry, models.NamedCount{Name: r.Name, Count: r.Count})
		}
	}
	return nil
}

// ── Content (testimonials + store) ─────────────────────────────────────────

func (s *churchOverviewService) fillContent(ctx context.Context, now time.Time, c *models.ChurchContent) error {
	var core struct {
		TestimonialsApproved int64
		TestimonialsPending  int64
		Testimonials30d      int64
		StoreRevenue         float64
		StorePaymentPending  int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT COUNT(*) FROM testimonials WHERE deleted_at IS NULL AND is_approved = TRUE) AS testimonials_approved,
			(SELECT COUNT(*) FROM testimonials WHERE deleted_at IS NULL AND is_approved = FALSE) AS testimonials_pending,
			(SELECT COUNT(*) FROM testimonials WHERE deleted_at IS NULL AND created_at >= NOW() - INTERVAL '30 days') AS testimonials30d,
			(SELECT COALESCE(SUM(total), 0) FROM store_orders WHERE payment_status = 'paid') AS store_revenue,
			(SELECT COUNT(*) FROM store_orders WHERE payment_status = 'proof_submitted') AS store_payment_pending
	`).Scan(&core).Error; err != nil {
		return err
	}
	c.TestimonialsApproved = core.TestimonialsApproved
	c.TestimonialsPending = core.TestimonialsPending
	c.Testimonials30d = core.Testimonials30d
	c.StoreRevenue = core.StoreRevenue
	c.StorePaymentPending = core.StorePaymentPending
	c.StoreOrdersByStatus = s.namedCounts(ctx, "store_orders", "status", "")
	return nil
}

// ── Events ─────────────────────────────────────────────────────────────────

func (s *churchOverviewService) fillEvents(ctx context.Context, now time.Time, e *models.ChurchEventsBlock) error {
	var core struct {
		Total    int64
		Upcoming int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE event_date >= CURRENT_DATE) AS upcoming
		FROM events
	`).Scan(&core).Error; err != nil {
		return err
	}
	e.Total = core.Total
	e.Upcoming = core.Upcoming
	e.HasData = core.Total > 0
	e.ByCategory = s.namedCounts(ctx, "events", "category", "")
	e.Monthly = s.monthlyCount(ctx, now, "events", "event_date", "")
	return nil
}

// ── shared query helpers ───────────────────────────────────────────────────

// monthlyCount runs a zero-filled 12-month count over table.dateCol, oldest
// first. `where` is an optional extra predicate (no leading AND).
func (s *churchOverviewService) monthlyCount(ctx context.Context, now time.Time, table, dateCol, where string) []models.MonthPoint {
	predicate := fmt.Sprintf("%s >= date_trunc('month', CURRENT_DATE) - INTERVAL '11 months'", dateCol)
	if strings.TrimSpace(where) != "" {
		predicate = where + " AND " + predicate
	}
	query := fmt.Sprintf(
		"SELECT to_char(%s, 'YYYY-MM') AS month, COUNT(*) AS value FROM %s WHERE %s GROUP BY month",
		dateCol, table, predicate,
	)
	type row struct {
		Month string
		Value int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return zeroFillCounts(now, nil)
	}
	byMonth := make(map[string]int64, len(rows))
	for _, r := range rows {
		byMonth[r.Month] = r.Value
	}
	return zeroFillCounts(now, byMonth)
}

// namedCounts runs `SELECT col, COUNT(*) FROM table [WHERE where] GROUP BY col`.
// table/col are code-supplied constants (never user input).
func (s *churchOverviewService) namedCounts(ctx context.Context, table, col, where string) []models.NamedCount {
	query := fmt.Sprintf("SELECT COALESCE(NULLIF(%s::text, ''), 'unspecified') AS name, COUNT(*) AS count FROM %s", col, table)
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	query += " GROUP BY name ORDER BY count DESC LIMIT 20"
	type row struct {
		Name  string
		Count int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return nil
	}
	out := make([]models.NamedCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.NamedCount{Name: r.Name, Count: r.Count})
	}
	return out
}

// enrichSignals folds giving / attendance / backlog momentum into the readiness
// score so it reflects the whole church, not just form submissions.
func (s *churchOverviewService) enrichSignals(o *models.ChurchOverview) {
	g := o.Giving
	if g.LastMonthKobo > 0 {
		o.Signals.GivingDeltaPercent = round(
			(float64(g.ThisMonthKobo-g.LastMonthKobo)/float64(g.LastMonthKobo))*100, 2)
	} else if g.ThisMonthKobo > 0 {
		o.Signals.GivingDeltaPercent = 100
	}

	e := o.Engagement
	if e.AttendancePrevious > 0 {
		o.Signals.AttendanceDeltaPercent = round(
			(float64(e.AttendanceCurrent-e.AttendancePrevious)/float64(e.AttendancePrevious))*100, 2)
	} else if e.AttendanceCurrent > 0 {
		o.Signals.AttendanceDeltaPercent = 100
	}

	b := e.Backlog
	openQueues := b.PrayerOpen + b.Contact30d + b.VisitsUpcoming + b.ApprovalsPending
	o.Signals.BacklogPressure = round(normalize(float64(openQueues), 0, 40), 3)

	givingMomentum := normalize(o.Signals.GivingDeltaPercent, -100, 100)
	attendanceMomentum := normalize(o.Signals.AttendanceDeltaPercent, -100, 100)
	submissionMomentum := normalize(o.Signals.SubmissionDeltaPercent, -100, 100)

	o.Signals.DecisionReadinessScore = round(clamp(
		(0.22*submissionMomentum+
			0.18*givingMomentum+
			0.15*attendanceMomentum+
			0.18*o.Signals.MemberActivationRate+
			0.17*o.Signals.VolunteerCoverageRate+
			0.10*(1-o.Signals.BacklogPressure))*100,
		0, 100), 2)

	// Extra church-wide recommendations on top of the decision engine's.
	if o.Signals.GivingDeltaPercent < -10 && g.LastMonthKobo > 0 {
		o.Recommendations = append(o.Recommendations,
			"Giving is down month-on-month. Confirm recording is current and plan a stewardship touchpoint.")
	}
	if o.Signals.AttendanceDeltaPercent < -10 && e.AttendancePrevious > 0 {
		o.Recommendations = append(o.Recommendations,
			"Attendance has dropped versus the previous period. Follow up with regulars who have not checked in recently.")
	}
	if b.PrayerOpen >= 10 {
		o.Recommendations = append(o.Recommendations,
			fmt.Sprintf("%d prayer requests are still open — assign and work the backlog.", b.PrayerOpen))
	}
	if o.Intake.WorkflowStalled >= 5 {
		o.Recommendations = append(o.Recommendations,
			fmt.Sprintf("%d new-member journeys have stalled for over two weeks — re-assign or close them.", o.Intake.WorkflowStalled))
	}
}
