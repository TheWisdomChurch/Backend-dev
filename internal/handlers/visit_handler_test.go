package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wisdomHouse-backend/internal/models"
)

func mustVisitDate(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02", raw, lagosLocation())
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestPublicVisitResponseDoesNotExposePII(t *testing.T) {
	visit := &models.VisitRequest{ID: "reference", Email: "private@example.com", Phone: "+234000", Notes: "private note"}
	payload, err := json.Marshal(publicVisit(visit))
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, secret := range []string{"private@example.com", "+234000", "private note"} {
		if strings.Contains(text, secret) {
			t.Fatalf("public response leaked %q", secret)
		}
	}
}

func TestVisitIdempotencyKeyIsServerOwned(t *testing.T) {
	base := CreateVisitRequest{Email: "Visitor@Example.com", ServiceDate: "2026-03-08", IdempotencyKey: "attacker-controlled"}
	first := visitIdempotencyKey(base)
	base.IdempotencyKey = "different-client-key"
	if second := visitIdempotencyKey(base); first != second {
		t.Fatalf("client idempotency key changed server-owned identity")
	}
	base.Email = "other@example.com"
	if other := visitIdempotencyKey(base); first == other {
		t.Fatalf("different visitor email produced the same identity")
	}
}

func TestVisitStatusTransitionsDoNotMoveBackwards(t *testing.T) {
	if visitStatusTransitions["contacted"]["confirmed"] {
		t.Fatal("contacted visit must not move backwards to confirmed")
	}
	if !visitStatusTransitions["arrived"]["completed"] {
		t.Fatal("arrived visit must be completable")
	}
	if len(visitStatusTransitions["completed"]) != 0 {
		t.Fatal("completed visit must be terminal")
	}
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
