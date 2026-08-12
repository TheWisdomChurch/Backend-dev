package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewPublicOrderIDIsOpaqueAndUnique(t *testing.T) {
	first := newPublicOrderID()
	second := newPublicOrderID()
	if first == second {
		t.Fatal("generated order IDs must be unique")
	}
	if !strings.HasPrefix(first, "ord_") {
		t.Fatalf("order ID %q has no ord_ prefix", first)
	}
	if _, err := uuid.Parse(strings.TrimPrefix(first, "ord_")); err != nil {
		t.Fatalf("order ID %q does not contain a valid UUID: %v", first, err)
	}
}
