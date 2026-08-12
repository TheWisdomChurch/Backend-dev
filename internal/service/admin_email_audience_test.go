package service

import (
	"reflect"
	"testing"

	"wisdomHouse-backend/internal/models"
)

func TestNormalizeAdminComposeRecipientsDeduplicatesCaseAndWhitespace(t *testing.T) {
	name := "Primary Name"
	items := []models.AdminEmailRecipientInput{
		{Email: " Person@Example.com ", Name: &name},
		{Email: "person@example.com"},
	}
	recipients, err := normalizeAdminComposeRecipients(&items)
	if err != nil {
		t.Fatalf("normalize recipients: %v", err)
	}
	if len(recipients) != 1 || recipients[0].Email != "person@example.com" || recipients[0].Name != name {
		t.Fatalf("unexpected recipients: %#v", recipients)
	}
}

func TestNormalizeAdminAudienceTypesValidatesAndDeduplicates(t *testing.T) {
	values := []string{" Workforce ", "members", "WORKFORCE", "subscribers"}
	got, err := normalizeAdminAudienceTypes(&values)
	if err != nil {
		t.Fatalf("normalize audience types: %v", err)
	}
	want := []string{"workforce", "members", "subscribers"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	invalid := []string{"all_database_users"}
	if _, err := normalizeAdminAudienceTypes(&invalid); err == nil {
		t.Fatal("expected unsupported audience type to be rejected")
	}
}
