package payment

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha512.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestPaystackValidateWebhook_AcceptsCorrectSignature(t *testing.T) {
	secret := "whsec_test_paystack"
	body := []byte(`{"event":"charge.success","data":{"reference":"REF-1"}}`)
	p := &paystackProvider{webhookSecret: secret}

	if err := p.ValidateWebhook(sign(secret, body), body); err != nil {
		t.Fatalf("expected valid signature to pass, got error: %v", err)
	}
}

func TestPaystackValidateWebhook_RejectsWrongSecret(t *testing.T) {
	body := []byte(`{"event":"charge.success"}`)
	p := &paystackProvider{webhookSecret: "whsec_correct"}

	if err := p.ValidateWebhook(sign("whsec_wrong", body), body); err == nil {
		t.Fatal("expected signature computed with the wrong secret to be rejected")
	}
}

func TestPaystackValidateWebhook_RejectsTamperedBody(t *testing.T) {
	secret := "whsec_test_paystack"
	original := []byte(`{"event":"charge.success","data":{"amount":1000}}`)
	tampered := []byte(`{"event":"charge.success","data":{"amount":9999999}}`)
	p := &paystackProvider{webhookSecret: secret}

	sig := sign(secret, original)
	if err := p.ValidateWebhook(sig, tampered); err == nil {
		t.Fatal("expected a signature for the original body to fail against a tampered body")
	}
}

func TestPaystackValidateWebhook_AcceptsUppercaseSignature(t *testing.T) {
	// Paystack signatures are lowercase hex, but the check should not be
	// case-sensitive to hex casing since ValidateWebhook lowercases input.
	secret := "whsec_test_paystack"
	body := []byte(`{"event":"charge.success"}`)
	p := &paystackProvider{webhookSecret: secret}

	upper := sign(secret, body)
	for i, r := range upper {
		if r >= 'a' && r <= 'f' {
			upper = upper[:i] + string(r-32) + upper[i+1:]
		}
	}

	if err := p.ValidateWebhook(upper, body); err != nil {
		t.Fatalf("expected uppercase-hex signature to still validate, got error: %v", err)
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"success":   "success",
		"Success":   "success",
		"failed":    "failed",
		"abandoned": "failed",
		"pending":   "pending",
		"unknown":   "pending",
		"":          "pending",
	}
	for in, want := range cases {
		if got := normalizeStatus(in); got != want {
			t.Errorf("normalizeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
