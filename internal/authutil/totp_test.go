package authutil

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildTOTPAuthURLUsesStandardLabelFormat(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	raw := BuildTOTPAuthURL("Wisdom House", "admin@example.com", secret)

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("expected valid otpauth url: %v", err)
	}

	if parsed.Scheme != "otpauth" {
		t.Fatalf("expected otpauth scheme, got %q", parsed.Scheme)
	}
	if parsed.Host != "totp" {
		t.Fatalf("expected totp host, got %q", parsed.Host)
	}
	if parsed.Path != "/Wisdom House:admin@example.com" {
		t.Fatalf("expected standard label path, got %q", parsed.Path)
	}

	query := parsed.Query()
	if query.Get("secret") != secret {
		t.Fatalf("expected secret query value to round-trip")
	}
	if query.Get("issuer") != "Wisdom House" {
		t.Fatalf("expected issuer query value to match")
	}
	if query.Get("digits") != "6" {
		t.Fatalf("expected 6 digits, got %q", query.Get("digits"))
	}
	if query.Get("period") != "30" {
		t.Fatalf("expected 30 second period, got %q", query.Get("period"))
	}
}

func TestBuildTOTPAuthURLSanitizesLabelSeparators(t *testing.T) {
	t.Parallel()

	raw := BuildTOTPAuthURL("Wisdom:House", "admin:ops@example.com", "JBSWY3DPEHPK3PXP")
	if strings.Contains(raw, "%3Aadmin") {
		t.Fatalf("expected literal issuer separator in label, got %q", raw)
	}
	if strings.Contains(raw, "Wisdom:House:admin:ops") {
		t.Fatalf("expected embedded colons to be stripped from label, got %q", raw)
	}
}

func TestVerifyTOTPGeneratedCodeWithSkew(t *testing.T) {
	t.Parallel()

	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0).UTC()

	code, err := generateTOTPCode(secret, now)
	if err != nil {
		t.Fatalf("expected code generation to succeed: %v", err)
	}

	if !VerifyTOTP(secret, code, now, 0) {
		t.Fatalf("expected current code to verify")
	}
	if !VerifyTOTP(secret, code, now.Add(DefaultTOTPPeriod), 1) {
		t.Fatalf("expected one-step skew to verify")
	}
	if VerifyTOTP(secret, code, now.Add(2*DefaultTOTPPeriod), 1) {
		t.Fatalf("expected code outside skew window to fail")
	}
}

func TestGenerateAndNormalizeRecoveryCodes(t *testing.T) {
	t.Parallel()

	codes, err := GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatalf("expected recovery codes generation to succeed: %v", err)
	}
	if len(codes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(codes))
	}

	seen := make(map[string]bool)
	for _, code := range codes {
		if len(code) != 9 || code[4] != '-' {
			t.Fatalf("unexpected recovery code format: %q", code)
		}
		norm := NormalizeRecoveryCode(code)
		if len(norm) != 8 {
			t.Fatalf("expected normalized length 8, got %d for %q", len(norm), norm)
		}
		if seen[norm] {
			t.Fatalf("expected unique recovery codes, found duplicate: %q", code)
		}
		seen[norm] = true

		if !LooksLikeRecoveryCode(code) {
			t.Fatalf("expected LooksLikeRecoveryCode to return true for %q", code)
		}
		if !LooksLikeRecoveryCode(norm) {
			t.Fatalf("expected LooksLikeRecoveryCode to return true for %q", norm)
		}
	}

	if LooksLikeRecoveryCode("123456") {
		t.Fatalf("6-digit TOTP should not look like recovery code")
	}
}

