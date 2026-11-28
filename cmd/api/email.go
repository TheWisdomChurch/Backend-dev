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

func (s observedEmailSender) SendHTMLTextWithOptions(to, subject, htmlBody, textBody string, opts email.MessageOptions) error {
	if s.inner == nil {
		return errors.New("email sender is not configured")
	}
	var err error
	if sender, ok := s.inner.(interface {
		SendHTMLTextWithOptions(string, string, string, string, email.MessageOptions) error
	}); ok {
		err = sender.SendHTMLTextWithOptions(to, subject, htmlBody, textBody, opts)
	} else {
		err = s.SendHTMLText(to, subject, htmlBody, textBody)
	}
	if err != nil {
		applog.L().Error("bulk email delivery failed", "to", maskEmail(to), "subject", subject, "error", err)
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

func (s observedEmailSender) SendHTMLWithAttachmentsAndOptions(to, subject, htmlBody, textBody string, attachments []email.Attachment, opts email.MessageOptions) error {
	if s.inner == nil {
		return errors.New("email sender is not configured")
	}
	if sender, ok := s.inner.(interface {
		SendHTMLWithAttachmentsAndOptions(string, string, string, string, []email.Attachment, email.MessageOptions) error
	}); ok {
		return sender.SendHTMLWithAttachmentsAndOptions(to, subject, htmlBody, textBody, attachments, opts)
	}
	return s.SendHTMLWithAttachments(to, subject, htmlBody, textBody, attachments)
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
		if s.fallback != nil && email.CanFallback(err) {
			applog.L().Warn("email send failed, falling back", "primary", s.primaryName, "fallback", s.fallbackName, "error", err)
			if err2 := s.fallback.SendHTML(to, subject, body); err2 == nil {
				return nil
			} else {
				return errors.Join(fmt.Errorf("%s failed: %w", s.primaryName, err), fmt.Errorf("%s failed: %w", s.fallbackName, err2))
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
		if s.fallback != nil && email.CanFallback(err) {
			applog.L().Warn("email send failed, falling back", "primary", s.primaryName, "fallback", s.fallbackName, "error", err)
			if err2 := sendMultipart(s.fallback); err2 == nil {
				return nil
			} else {
				return errors.Join(fmt.Errorf("%s failed: %w", s.primaryName, err), fmt.Errorf("%s failed: %w", s.fallbackName, err2))
			}
		}
		return err
	}
	return nil
}

func (s chainedEmailSender) SendHTMLTextWithOptions(to, subject, htmlBody, textBody string, opts email.MessageOptions) error {
	send := func(sender service.EmailSender) error {
		if sender == nil {
			return errors.New("email sender is not configured")
		}
		if capable, ok := sender.(interface {
			SendHTMLTextWithOptions(string, string, string, string, email.MessageOptions) error
		}); ok {
			return capable.SendHTMLTextWithOptions(to, subject, htmlBody, textBody, opts)
		}
		if multipart, ok := sender.(interface {
			SendHTMLText(string, string, string, string) error
		}); ok {
			return multipart.SendHTMLText(to, subject, htmlBody, textBody)
		}
		return sender.SendHTML(to, subject, htmlBody)
	}
	if s.primary == nil {
		return send(s.fallback)
	}
	if err := send(s.primary); err != nil && s.fallback != nil && email.CanFallback(err) {
		if fallbackErr := send(s.fallback); fallbackErr != nil {
			return errors.Join(fmt.Errorf("%s failed: %w", s.primaryName, err), fmt.Errorf("%s failed: %w", s.fallbackName, fallbackErr))
		}
		return nil
	} else {
		return err
	}
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
		if s.fallback != nil && email.CanFallback(err) {
			applog.L().Warn("email send with attachments failed, falling back", "primary", s.primaryName, "fallback", s.fallbackName, "error", err)
			if err2 := sendWithAttachments(s.fallback); err2 == nil {
				return nil
			} else {
				return errors.Join(fmt.Errorf("%s failed: %w", s.primaryName, err), fmt.Errorf("%s failed: %w", s.fallbackName, err2))
			}
		}
		return err
	}
	return nil
}

func (s chainedEmailSender) SendHTMLWithAttachmentsAndOptions(to, subject, htmlBody, textBody string, attachments []email.Attachment, opts email.MessageOptions) error {
	send := func(sender service.EmailSender) error {
		if sender == nil {
			return errors.New("email sender is not configured")
		}
		if capable, ok := sender.(interface {
			SendHTMLWithAttachmentsAndOptions(string, string, string, string, []email.Attachment, email.MessageOptions) error
		}); ok {
			return capable.SendHTMLWithAttachmentsAndOptions(to, subject, htmlBody, textBody, attachments, opts)
		}
		if capable, ok := sender.(interface {
			SendHTMLWithAttachments(string, string, string, string, []email.Attachment) error
		}); ok {
			return capable.SendHTMLWithAttachments(to, subject, htmlBody, textBody, attachments)
		}
		return errors.New("email sender does not support attachments")
	}
	if s.primary == nil {
		return send(s.fallback)
	}
	if err := send(s.primary); err != nil && s.fallback != nil && email.CanFallback(err) {
		if fallbackErr := send(s.fallback); fallbackErr != nil {
			return errors.Join(fmt.Errorf("%s failed: %w", s.primaryName, err), fmt.Errorf("%s failed: %w", s.fallbackName, fallbackErr))
		}
		return nil
	} else {
		return err
	}
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

	var mailcow service.EmailSender
	var brevo service.EmailSender

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
		} else {
			mailcow = s
			applog.L().Info("email sender initialized (Mailcow SMTP submission)")
		}
	}

	if hasAnyEnv("BREVO_API_KEY", "BREVO_FROM_EMAIL", "BREVO_FROM_NAME", "BREVO_BASE_URL") {
		s, err := email.NewBrevoSender(cfg.Redis.URL, "", "", "", "")
		if err != nil {
			applog.L().Warn("Brevo email sender not initialized", "error", err)
		} else {
			brevo = s
			applog.L().Info("Brevo email fallback initialized")
		}
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.SMTP.PrimaryProvider))
	if provider == "" {
		provider = "mailcow"
	}
	switch provider {
	case "mailcow", "smtp":
		primary, primaryName = mailcow, "Mailcow"
		fallback, fallbackName = brevo, "Brevo"
	case "brevo":
		primary, primaryName = brevo, "Brevo"
		fallback, fallbackName = mailcow, "Mailcow"
	default:
		applog.L().Error("invalid email primary provider; email sending disabled", "provider", provider)
		return noopEmailSender{}
	}

	if primary == nil && fallback != nil {
		applog.L().Warn("configured primary email provider unavailable; using secondary provider", "provider", provider, "secondary", fallbackName)
		primary, primaryName = fallback, fallbackName
		fallback, fallbackName = nil, ""
	}
	if primary == nil {
		applog.L().Warn("email sender not configured (Mailcow SMTP or Brevo API); outbound email disabled")
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
