package handlers

import "testing"

func TestOrderLookupAuthorized(t *testing.T) {
	tests := []struct {
		name, provided, stored string
		want                   bool
	}{
		{name: "exact", provided: "visitor@example.com", stored: "visitor@example.com", want: true},
		{name: "normalized", provided: " VISITOR@Example.com ", stored: "visitor@example.com", want: true},
		{name: "missing", stored: "visitor@example.com", want: false},
		{name: "different", provided: "attacker@example.com", stored: "visitor@example.com", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := orderLookupAuthorized(test.provided, test.stored); got != test.want {
				t.Fatalf("orderLookupAuthorized() = %v, want %v", got, test.want)
			}
		})
	}
}
