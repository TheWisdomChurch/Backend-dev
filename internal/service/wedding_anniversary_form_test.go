package service

import (
	"testing"

	"wisdomHouse-backend/internal/models"
)

func TestBuildWeddingAnniversaryInput_NoDateMeansNotPresent(t *testing.T) {
	if _, ok := buildWeddingAnniversaryInput(map[string]any{"firstName": "Peter"}); ok {
		t.Fatal("expected ok=false when no anniversary field is present")
	}
}

func TestBuildWeddingAnniversaryInput_MapsRecognisedKeys(t *testing.T) {
	values := map[string]any{
		"weddingAnniversary": "14/06",
		"spouseName":         "sarah ogba",
		"spouseEmail":        "Sarah@Example.com",
		"spouseEmailConsent": "yes",
		"spouseIsExternal":   "true",
	}
	in, ok := buildWeddingAnniversaryInput(values)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if in.Anniversary == nil || *in.Anniversary != "14/06" {
		t.Fatalf("anniversary: got %v", in.Anniversary)
	}
	if in.SpouseName != "sarah ogba" {
		t.Fatalf("spouse name: got %q", in.SpouseName)
	}
	if in.SpouseEmail == nil || *in.SpouseEmail != "Sarah@Example.com" {
		t.Fatalf("spouse email: got %v", in.SpouseEmail)
	}
	if !in.SpouseEmailConsent {
		t.Fatal("expected consent true")
	}
	if in.SpouseIsExternal == nil || !*in.SpouseIsExternal {
		t.Fatal("expected spouseIsExternal true")
	}
}

func TestBuildWeddingAnniversaryInput_NoConsentWithoutExplicitYes(t *testing.T) {
	values := map[string]any{
		"anniversary":  "01/12",
		"spouse_email": "person@example.com",
	}
	in, ok := buildWeddingAnniversaryInput(values)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if in.SpouseEmailConsent {
		t.Fatal("consent must default to false when not explicitly given")
	}
}

func TestCoupleGreetingName(t *testing.T) {
	v := models.WeddingAnniversaryView{
		WeddingAnniversary: models.WeddingAnniversary{SpouseName: "sarah"},
		FirstName:          "david",
		LastName:           "ogba",
	}
	if got := coupleGreetingName(v); got != "David Ogba & Sarah" {
		t.Fatalf("got %q", got)
	}

	v.SpouseName = ""
	if got := coupleGreetingName(v); got != "David Ogba" {
		t.Fatalf("no-spouse case: got %q", got)
	}
}

func TestCoupleAddresses_RequiresConsentForSpouse(t *testing.T) {
	spouseEmail := "spouse@example.com"
	v := models.WeddingAnniversaryView{
		WeddingAnniversary: models.WeddingAnniversary{SpouseEmail: &spouseEmail, SpouseEmailConsent: false},
		Email:              "member@example.com",
	}
	addrs := coupleAddresses(v)
	if len(addrs) != 1 || addrs[0] != "member@example.com" {
		t.Fatalf("expected member-only without consent, got %v", addrs)
	}

	v.SpouseEmailConsent = true
	addrs = coupleAddresses(v)
	if len(addrs) != 2 {
		t.Fatalf("expected both addresses with consent, got %v", addrs)
	}
}
