package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/email"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/service"
)

type chainedEmailSender struct {
	primary      service.EmailSender
	fallback     service.EmailSender
	primaryName  string
	fallbackName string
}

type observedEmailSender struct {
	inner service.EmailSender
}

func (s observedEmailSender) SendHTML(to, subject, body string) error {
	if s.inner == nil {
		return nil
	}
	err := s.inner.SendHTML(to, subject, body)
	if err != nil {
		applog.L().Error("email delivery failed", "to", maskEmail(to), "subject", subject, "error", err)
	}
	return err
}

func (s observedEmailSender) SendHTMLText(to, subject, htmlBody, textBody string) error {
	if s.inner == nil {
		return nil
	}
	if multipart, ok := s.inner.(interface {
		SendHTMLText(to, subject, htmlBody, textBody string) error
	}); ok {
		err := multipart.SendHTMLText(to, subject, htmlBody, textBody)
		if err != nil {
			applog.L().Error("multipart email delivery failed", "to", maskEmail(to), "subject", subject, "error", err)
		}
		return err
	}
	err := s.inner.SendHTML(to, subject, htmlBody)
	if err != nil {
		applog.L().Error("HTML email delivery failed", "to", maskEmail(to), "subject", subject, "error", err)
	}
	return err
}

func (s observedEmailSender) FetchAttachment(ctx context.Context, fileURL, filename string) (email.Attachment, error) {
	if s.inner == nil {
		return email.Attachment{}, errors.New("email sender is not configured")
	}
	fetcher, ok := s.inner.(interface {
		FetchAttachment(ctx context.Context, fileURL, filename string) (email.Attachment, error)
	})
	if !ok {
		return email.Attachment{}, errors.New("email sender does not support attachments")
	}
	return fetcher.FetchAttachment(ctx, fileURL, filename)
}

func (s observedEmailSender) SendHTMLWithAttachments(to, subject, htmlBody, textBody string, attachments []email.Attachment) error {
	if s.inner == nil {
		return nil
	}
	withAttachments, ok := s.inner.(interface {
		SendHTMLWithAttachments(to, subject, htmlBody, textBody string, attachments []email.Attachment) error
	})
	if !ok {
		return errors.New("email sender does not support attachments")
	}
	err := withAttachments.SendHTMLWithAttachments(to, subject, htmlBody, textBody, attachments)
	if err != nil {
		applog.L().Error("email delivery with attachments failed", "to", maskEmail(to), "subject", subject, "error", err)
	}
	return err
}

func maskEmail(raw string) string {
	e := strings.TrimSpace(strings.ToLower(raw))
	parts := strings.Split(e, "@")
	if len(parts) != 2 {
		return "invalid-email"
	}
	local := parts[0]
	domain := parts[1]
	if local == "" {
		return "***@" + domain
	}
	if len(local) <= 2 {
		return local[:1] + "***@" + domain
	}
	return local[:2] + "***@" + domain
}

func (s chainedEmailSender) SendHTML(to, subject, body string) error {
	if s.primary == nil {
		if s.fallback != nil {
			return s.fallback.SendHTML(to, subject, body)
		}
		return nil
	}
	if err := s.primary.SendHTML(to, subject, body); err != nil {
		if s.fallback != nil {
			applog.L().Warn("email send failed, falling back", "primary", s.primaryName, "fallback", s.fallbackName, "error", err)
			if err2 := s.fallback.SendHTML(to, subject, body); err2 == nil {
				return nil
			} else {
				return fmt.Errorf("%s failed: %w; %s failed: %v", s.primaryName, err, s.fallbackName, err2)
			}
		}
		return err
	}
	return nil
}

func (s chainedEmailSender) SendHTMLText(to, subject, htmlBody, textBody string) error {
	sendMultipart := func(sender service.EmailSender) error {
		if sender == nil {
			return nil
		}
		if multipart, ok := sender.(interface {
			SendHTMLText(to, subject, htmlBody, textBody string) error
		}); ok {
			return multipart.SendHTMLText(to, subject, htmlBody, textBody)
		}
		return sender.SendHTML(to, subject, htmlBody)
	}

	if s.primary == nil {
		if s.fallback != nil {
			return sendMultipart(s.fallback)
		}
		return nil
	}
	if err := sendMultipart(s.primary); err != nil {
		if s.fallback != nil {
			applog.L().Warn("email send failed, falling back", "primary", s.primaryName, "fallback", s.fallbackName, "error", err)
			if err2 := sendMultipart(s.fallback); err2 == nil {
				return nil
			} else {
				return fmt.Errorf("%s failed: %w; %s failed: %v", s.primaryName, err, s.fallbackName, err2)
			}
		}
		return err
	}
	return nil
}

func (s chainedEmailSender) SendHTMLWithAttachments(to, subject, htmlBody, textBody string, attachments []email.Attachment) error {
	sendWithAttachments := func(sender service.EmailSender) error {
		if sender == nil {
			return errors.New("email sender is not configured")
		}
		withAttachments, ok := sender.(interface {
			SendHTMLWithAttachments(to, subject, htmlBody, textBody string, attachments []email.Attachment) error
		})
		if !ok {
			return errors.New("email sender does not support attachments")
		}
		return withAttachments.SendHTMLWithAttachments(to, subject, htmlBody, textBody, attachments)
	}

	if s.primary == nil {
		if s.fallback != nil {
			return sendWithAttachments(s.fallback)
		}
		return errors.New("email sender is not configured")
	}
	if err := sendWithAttachments(s.primary); err != nil {
		if s.fallback != nil {
			applog.L().Warn("email send with attachments failed, falling back", "primary", s.primaryName, "fallback", s.fallbackName, "error", err)
			if err2 := sendWithAttachments(s.fallback); err2 == nil {
				return nil
			} else {
				return fmt.Errorf("%s failed: %w; %s failed: %v", s.primaryName, err, s.fallbackName, err2)
			}
		}
		return err
	}
	return nil
}

func (s chainedEmailSender) FetchAttachment(ctx context.Context, fileURL, filename string) (email.Attachment, error) {
	if s.primary != nil {
		if fetcher, ok := s.primary.(interface {
			FetchAttachment(ctx context.Context, fileURL, filename string) (email.Attachment, error)
		}); ok {
			return fetcher.FetchAttachment(ctx, fileURL, filename)
		}
	}
	if s.fallback != nil {
		if fetcher, ok := s.fallback.(interface {
			FetchAttachment(ctx context.Context, fileURL, filename string) (email.Attachment, error)
		}); ok {
			return fetcher.FetchAttachment(ctx, fileURL, filename)
		}
	}
	return email.Attachment{}, errors.New("email sender does not support attachments")
}

func initEmailSender(cfg *config.Config) service.EmailSender {
	if cfg == nil {
		return noopEmailSender{}
	}

	var primary service.EmailSender
	var fallback service.EmailSender
	var primaryName string
	var fallbackName string

	// Brevo goes first when configured: it's the fully-authenticated path
	// (SPF + DKIM both verified against wisdomchurchhq.org's DNS). The raw
	// SMTP relay only picks up SPF via the domain's "mx" rule and has no
	// DKIM signing of its own, so mail sent through it as primary was
	// passing SMTP delivery (the app saw "sent") while still getting
	// silently spam-filtered or dropped by the receiving provider.
	if hasAnyEnv("BREVO_API_KEY", "BREVO_FROM_EMAIL", "BREVO_FROM_NAME", "BREVO_BASE_URL") {
		s, err := email.NewBrevoSender(cfg.Redis.URL, "", "", "", "")
		if err != nil {
			applog.L().Warn("Brevo email sender not initialized", "error", err)
		} else {
			primary = s
			primaryName = "Brevo"
			applog.L().Info("email sender initialized (Brevo API)")
		}
	}

	if strings.TrimSpace(cfg.SMTP.Host) != "" {
		s, err := email.NewSender(
			cfg.Redis.URL,
			cfg.SMTP.Host,
			cfg.SMTP.Port,
			cfg.SMTP.User,
			cfg.SMTP.Password,
			cfg.SMTP.From,
			cfg.SMTP.TLS,
		)
		if err != nil {
			applog.L().Warn("SMTP sender not initialized", "error", err)
		} else if primary == nil {
			primary = s
			primaryName = "SMTP"
			applog.L().Info("email sender initialized (SMTP relay)")
		} else {
			fallback = s
			fallbackName = "SMTP"
			applog.L().Info("email fallback configured (SMTP relay)")
		}
	}

	if primary == nil {
		applog.L().Warn("email sender not configured (no SMTP/Brevo/SES); outbound email disabled")
		return noopEmailSender{}
	}
	if fallback == nil {
		return primary
	}
	return chainedEmailSender{
		primary:      primary,
		fallback:     fallback,
		primaryName:  primaryName,
		fallbackName: fallbackName,
	}
}

type noopEmailSender struct{}

func (noopEmailSender) SendHTML(string, string, string) error {
	return errors.New("outbound email sending is disabled or not configured")
}

func (noopEmailSender) SendHTMLText(string, string, string, string) error {
	return errors.New("outbound email sending is disabled or not configured")
}

func (noopEmailSender) DisabledReason() string {
	return "outbound email sending is disabled or not configured"
}
