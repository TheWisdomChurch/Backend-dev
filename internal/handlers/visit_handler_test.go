package handlers

import (
	"testing"
	"time"
)

func mustVisitDate(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02", raw, lagosLocation())
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestClassifySunday(t *testing.T) {
	tests := []struct{ date, want string }{
		{"2026-03-01", "Celebration & Communion Service"},
		{"2026-03-08", "Gaining Wisdom Service"},
		{"2026-03-15", "Gaining Wisdom Service"},
		{"2026-03-22", "Gaining Wisdom Service"},
		{"2026-03-29", "Supernatural Service"},
		{"2026-04-26", "Supernatural Service"},
	}
	for _, test := range tests {
		t.Run(test.date, func(t *testing.T) {
			if got := classifySunday(mustVisitDate(t, test.date)); got != test.want {
				t.Fatalf("classifySunday(%s) = %q, want %q", test.date, got, test.want)
			}
		})
	}
}
