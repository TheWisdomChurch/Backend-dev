package config

import (
	"strings"
	"testing"
	"time"
)

// validConfig returns a Config that passes validateConfig in a non-production
// environment (SMTP and Google OAuth left unconfigured so those optional
// validation branches are skipped).
func validConfig() *Config {
	return &Config{
		App: AppConfig{Environment: "development"},
		JWT: JWTConfig{Secret: strings.Repeat("a", 32)},
		Auth: AuthConfig{
			SessionIdleTimeout:           30 * time.Minute,
			RememberedSessionIdleTimeout: 720 * time.Hour,
			RememberMeTTL:                720 * time.Hour,
			CSRFCookieTTL:                12 * time.Hour,
			CSRFCookieName:               "csrf_secret",
			CSRFHeaderName:               "X-CSRF-Token",
			SecretKey:                    strings.Repeat("b", 32),
			MFAIssuer:                    "Test Issuer",
		},
		Server: ServerConfig{RequestBodyMax: 1 << 20},
	}
}

func TestValidateConfig_AcceptsMinimalValidConfig(t *testing.T) {
	if err := validateConfig(validConfig()); err != nil {
		t.Fatalf("expected a minimal valid config to pass, got error: %v", err)
	}
}

func TestValidateConfig_RejectsShortJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWT.Secret = strings.Repeat("a", 31)

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected a 31-character JWT_SECRET to be rejected")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Errorf("error = %q, want it to mention JWT_SECRET", err.Error())
	}
}

func TestValidateConfig_RejectsEmptyJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWT.Secret = ""

	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected an empty JWT_SECRET to be rejected")
	}
}

func TestValidateConfig_AcceptsExactly32CharJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWT.Secret = strings.Repeat("a", 32)

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected an exactly-32-character JWT_SECRET to pass, got: %v", err)
	}
}

func TestValidateConfig_RejectsShortAuthSecretKey(t *testing.T) {
	cfg := validConfig()
	cfg.Auth.SecretKey = strings.Repeat("b", 31)

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected a 31-character AUTH_SECRET_KEY to be rejected")
	}
	if !strings.Contains(err.Error(), "AUTH_SECRET_KEY") {
		t.Errorf("error = %q, want it to mention AUTH_SECRET_KEY", err.Error())
	}
}

func TestLoadDefaultsMailcowToSubmissionPortAndTLS(t *testing.T) {
	for _, key := range []string{"MAILCOW_PORT", "SMTP_PORT", "APP_SMTP_PORT", "MAIL_PORT", "SMTP_TLS", "APP_SMTP_TLS", "MAIL_TLS", "MAILCOW_TLS"} {
		t.Setenv(key, "")
	}
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("JWT_SECRET", strings.Repeat("j", 32))
	t.Setenv("AUTH_SECRET_KEY", strings.Repeat("a", 32))
	t.Setenv("MAILCOW_HOST", "mail.example.org")
	t.Setenv("MAILCOW_USER", "mailer@example.org")
	t.Setenv("MAILCOW_PASS", "test-password")
	t.Setenv("MAILCOW_FROM", "mailer@example.org")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SMTP.Port != "587" || !cfg.SMTP.TLS {
		t.Fatalf("Mailcow submission config = port %q TLS %v; want 587/true", cfg.SMTP.Port, cfg.SMTP.TLS)
	}
	if cfg.SMTP.PrimaryProvider != "mailcow" {
		t.Fatalf("primary provider = %q, want mailcow", cfg.SMTP.PrimaryProvider)
	}
}

func TestValidateConfigRejectsUnknownEmailProvider(t *testing.T) {
	cfg := validConfig()
	cfg.SMTP.PrimaryProvider = "unknown"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "EMAIL_PRIMARY_PROVIDER") {
		t.Fatalf("validateConfig() error = %v, want invalid EMAIL_PRIMARY_PROVIDER error", err)
	}
}
