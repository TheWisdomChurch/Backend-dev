package main

import (
	"errors"
	"testing"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/email"
)

type recordingEmailSender struct {
	calls int
	err   error
}

func (s *recordingEmailSender) SendHTML(string, string, string) error {
	s.calls++
	return s.err
}

func TestChainedEmailSenderFailsOverOnlyOnDefinitiveRejection(t *testing.T) {
	tests := []struct {
		name         string
		primaryError error
		wantFallback int
		wantErr      bool
	}{
		{name: "accepted primary does not call fallback", wantFallback: 0},
		{name: "ambiguous primary error does not risk duplicate", primaryError: errors.New("connection lost while waiting for response"), wantFallback: 0, wantErr: true},
		{name: "definite primary rejection uses fallback", primaryError: email.MarkFallbackEligible(errors.New("SMTP 451 temporary rejection")), wantFallback: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &recordingEmailSender{err: tt.primaryError}
			fallback := &recordingEmailSender{}
			sender := chainedEmailSender{primary: primary, fallback: fallback, primaryName: "Mailcow", fallbackName: "Brevo"}

			err := sender.SendHTML("person@example.org", "subject", "body")
			if (err != nil) != tt.wantErr {
				t.Fatalf("SendHTML error = %v, wantErr %v", err, tt.wantErr)
			}
			if fallback.calls != tt.wantFallback {
				t.Fatalf("fallback calls = %d, want %d", fallback.calls, tt.wantFallback)
			}
		})
	}
}

func TestChainedEmailSenderReturnsBothErrorsWhenFallbackFails(t *testing.T) {
	primaryErr := email.MarkFallbackEligible(errors.New("Mailcow rejected message"))
	fallbackErr := errors.New("Brevo unavailable")
	primary := &recordingEmailSender{err: primaryErr}
	fallback := &recordingEmailSender{err: fallbackErr}
	sender := chainedEmailSender{primary: primary, fallback: fallback, primaryName: "Mailcow", fallbackName: "Brevo"}

	err := sender.SendHTML("person@example.org", "subject", "body")
	if err == nil {
		t.Fatal("expected both provider errors")
	}
	if !errors.Is(err, primaryErr) || !errors.Is(err, fallbackErr) {
		t.Fatalf("error %v should wrap both provider errors", err)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("calls = primary %d, fallback %d; want one each", primary.calls, fallback.calls)
	}
}

func TestInitEmailSenderUsesMailcowPrimaryAndBrevoFallback(t *testing.T) {
	t.Setenv("BREVO_API_KEY", "test-key")
	t.Setenv("BREVO_FROM_EMAIL", "sender@example.org")
	t.Setenv("BREVO_FROM_NAME", "Test")
	t.Setenv("BREVO_BASE_URL", "")
	cfg := &config.Config{}
	cfg.SMTP.Host = "mail.example.org"
	cfg.SMTP.Port = "587"
	cfg.SMTP.User = "sender@example.org"
	cfg.SMTP.Password = "test-password"
	cfg.SMTP.From = "sender@example.org"
	cfg.SMTP.TLS = true
	cfg.SMTP.PrimaryProvider = "mailcow"

	sender, ok := initEmailSender(cfg).(chainedEmailSender)
	if !ok {
		t.Fatalf("initEmailSender() returned %T, want chainedEmailSender", initEmailSender(cfg))
	}
	if _, ok := sender.primary.(*email.Sender); !ok {
		t.Fatalf("primary provider is %T, want Mailcow SMTP sender", sender.primary)
	}
	if _, ok := sender.fallback.(*email.BrevoSender); !ok {
		t.Fatalf("fallback provider is %T, want Brevo sender", sender.fallback)
	}
}
