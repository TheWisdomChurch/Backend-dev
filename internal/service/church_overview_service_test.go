package service

import (
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
)

func TestNormalizeRange(t *testing.T) {
	cases := map[string]string{
		"month": "month", "MONTH": "month", " year ": "year",
		"last30": "last30", "": "last30", "garbage": "last30",
	}
	for in, want := range cases {
		if got := normalizeRange(in); got != want {
			t.Errorf("normalizeRange(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveWindow(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	cs, ce, ps, pe := resolveWindow("month", now)
	if cs != time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) || ce != now {
		t.Errorf("month current window wrong: %v..%v", cs, ce)
	}
	if ps != time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) || pe != cs {
		t.Errorf("month previous window wrong: %v..%v", ps, pe)
	}

	cs, _, ps, pe = resolveWindow("year", now)
	if cs != time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) || ps.Year() != 2025 || pe != cs {
		t.Errorf("year window wrong: cur=%v prev=%v..%v", cs, ps, pe)
	}

	cs, ce, ps, pe = resolveWindow("last30", now)
	if ce != now || !cs.Equal(now.AddDate(0, 0, -30)) || !ps.Equal(now.AddDate(0, 0, -60)) || pe != cs {
		t.Errorf("last30 window wrong: %v..%v / %v..%v", cs, ce, ps, pe)
	}
}

func TestMonthKeysAndZeroFill(t *testing.T) {
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	keys := monthKeys(now)
	if len(keys) != 12 {
		t.Fatalf("expected 12 keys, got %d", len(keys))
	}
	if keys[0] != "2025-04" || keys[11] != "2026-03" {
		t.Errorf("month key bounds wrong: first=%s last=%s", keys[0], keys[11])
	}

	pts := zeroFillCounts(now, map[string]int64{"2026-03": 7, "2025-12": 3})
	if len(pts) != 12 || pts[11].Value != 7 || pts[8].Value != 3 || pts[0].Value != 0 {
		t.Errorf("zeroFillCounts wrong: %+v", pts)
	}

	money := zeroFillMoney(now, map[string]int64{"2026-02": 150000})
	if len(money) != 12 || money[10].AmountKobo != 150000 || money[0].AmountKobo != 0 {
		t.Errorf("zeroFillMoney wrong: %+v", money)
	}
}

func TestEnrichSignalsScoreInRange(t *testing.T) {
	o := &models.ChurchOverview{
		Giving: models.ChurchGiving{ThisMonthKobo: 80000, LastMonthKobo: 100000},
		Engagement: models.ChurchEngagement{
			AttendanceCurrent: 120, AttendancePrevious: 200,
			Backlog: models.ChurchBacklog{PrayerOpen: 12, Contact30d: 4, VisitsUpcoming: 2, ApprovalsPending: 1},
		},
		Signals: models.DecisionSignalMetrics{
			MemberActivationRate: 0.7, VolunteerCoverageRate: 0.5, SubmissionDeltaPercent: -20,
		},
	}
	(&churchOverviewService{}).enrichSignals(o)

	if o.Signals.GivingDeltaPercent != -20 {
		t.Errorf("giving delta = %v, want -20", o.Signals.GivingDeltaPercent)
	}
	if o.Signals.AttendanceDeltaPercent != -40 {
		t.Errorf("attendance delta = %v, want -40", o.Signals.AttendanceDeltaPercent)
	}
	if o.Signals.BacklogPressure <= 0 || o.Signals.BacklogPressure > 1 {
		t.Errorf("backlog pressure out of range: %v", o.Signals.BacklogPressure)
	}
	if o.Signals.DecisionReadinessScore < 0 || o.Signals.DecisionReadinessScore > 100 {
		t.Errorf("readiness score out of range: %v", o.Signals.DecisionReadinessScore)
	}
	// giving down >10%, attendance down >10%, prayer backlog >= 10 → 3 extra recs
	if len(o.Recommendations) < 3 {
		t.Errorf("expected church-wide recommendations, got %v", o.Recommendations)
	}
}

func TestEnrichSignalsNoBaseline(t *testing.T) {
	o := &models.ChurchOverview{
		Giving:     models.ChurchGiving{ThisMonthKobo: 5000, LastMonthKobo: 0},
		Engagement: models.ChurchEngagement{AttendanceCurrent: 40, AttendancePrevious: 0},
	}
	(&churchOverviewService{}).enrichSignals(o)
	if o.Signals.GivingDeltaPercent != 100 || o.Signals.AttendanceDeltaPercent != 100 {
		t.Errorf("no-baseline deltas should be 100: giving=%v attendance=%v",
			o.Signals.GivingDeltaPercent, o.Signals.AttendanceDeltaPercent)
	}
}
