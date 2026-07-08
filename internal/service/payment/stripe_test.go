package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func stripeSignatureHeader(secret, timestamp string, body []byte) string {
	payload := timestamp + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", timestamp, sig)
}

func TestStripeValidateWebhook_AcceptsCorrectSignature(t *testing.T) {
	secret := "whsec_test_stripe"
	body := []byte(`{"type":"checkout.session.completed"}`)
	p := &stripeProvider{webhookSecret: secret}

	header := stripeSignatureHeader(secret, "1700000000", body)
	if err := p.ValidateWebhook(header, body); err != nil {
		t.Fatalf("expected valid signature to pass, got error: %v", err)
	}
}

func TestStripeValidateWebhook_RejectsWrongSecret(t *testing.T) {
	body := []byte(`{"type":"checkout.session.completed"}`)
	p := &stripeProvider{webhookSecret: "whsec_correct"}

	header := stripeSignatureHeader("whsec_wrong", "1700000000", body)
	if err := p.ValidateWebhook(header, body); err == nil {
		t.Fatal("expected signature computed with the wrong secret to be rejected")
	}
}

func TestStripeValidateWebhook_RejectsTamperedBody(t *testing.T) {
	secret := "whsec_test_stripe"
	original := []byte(`{"amount":1000}`)
	tampered := []byte(`{"amount":9999999}`)
	p := &stripeProvider{webhookSecret: secret}

	header := stripeSignatureHeader(secret, "1700000000", original)
	if err := p.ValidateWebhook(header, tampered); err == nil {
		t.Fatal("expected a signature for the original body to fail against a tampered body")
	}
}

func TestStripeValidateWebhook_RejectsMalformedHeader(t *testing.T) {
	p := &stripeProvider{webhookSecret: "whsec_test_stripe"}
	body := []byte(`{}`)

	cases := []string{
		"",
		"not-a-valid-header",
		"t=1700000000", // missing v1
		"v1=deadbeef",  // missing t
	}
	for _, header := range cases {
		if err := p.ValidateWebhook(header, body); err == nil {
			t.Errorf("expected malformed header %q to be rejected", header)
		}
	}
}

func TestStripeValidateWebhook_AcceptsAnyMatchingSignatureWhenMultiplePresent(t *testing.T) {
	// Stripe can send multiple v1 signatures during secret rotation; a match
	// on any one of them should be accepted.
	secret := "whsec_test_stripe"
	body := []byte(`{"type":"checkout.session.completed"}`)
	p := &stripeProvider{webhookSecret: secret}

	valid := stripeSignatureHeader(secret, "1700000000", body)
	// Prepend a bogus v1 signature ahead of the valid one.
	header := "t=1700000000,v1=deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef," + valid[len("t=1700000000,"):]

	if err := p.ValidateWebhook(header, body); err != nil {
		t.Fatalf("expected at least one matching v1 signature to validate, got error: %v", err)
	}
}
