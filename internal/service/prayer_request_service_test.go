package service

import (
	"strings"
	"testing"
)

func TestNewPrayerRequestServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := NewPrayerRequestService(nil, "valid-secret"); err == nil || !strings.Contains(err.Error(), "repository") {
		t.Fatalf("expected repository configuration error, got %v", err)
	}
}
