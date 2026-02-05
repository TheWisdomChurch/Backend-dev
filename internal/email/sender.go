// internal/email/sender.go
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	gomail "github.com/wneessen/go-mail"
)

// Sender is the central SMTP sender used by all services.
// It supports:
//
//   • Local relay (Postfix) without SMTP auth
//   • Direct SMTP providers (Brevo, Gmail, etc.) with auth
//   • Simple per-recipient rate limiting via Redis
type Sender struct {
	host       string
	port       int
	user       string
	pass       string
	from       string
	requireTLS bool
	timeout    time.Duration
	redis      *redis.Client
}

// NewSender builds a Sender.
// Arguments come from config, but are allowed to be empty: we then fall back to env.
func NewSender(redisURL, host, port, user, pass, from string, requireTLS bool) (*Sender, error) {
	// Normalize / fall back to env
	h := strings.TrimSpace(host)
	if h == "" {
		h = strings.TrimSpace(os.Getenv("SMTP_HOST"))
		if h == "" {
			h = strings.TrimSpace(os.Getenv("APP_SMTP_HOST"))
		}
	}

	p := strings.TrimSpace(port)
	if p == "" {
		p = strings.TrimSpace(os.Getenv("SMTP_PORT"))
		if p == "" {
			p = strings.TrimSpace(os.Getenv("APP_SMTP_PORT"))
		}
	}
	if p == "" {
		p = "25"
	}

	u := strings.TrimSpace(user)
	if u == "" {
		u = strings.TrimSpace(os.Getenv("SMTP_USER"))
		if u == "" {
			u = strings.TrimSpace(os.Getenv("APP_SMTP_USER"))
		}
	}

	pw := strings.TrimSpace(pass)
	if pw == "" {
		pw = strings.TrimSpace(os.Getenv("SMTP_PASS"))
		if pw == "" {
			pw = strings.TrimSpace(os.Getenv("APP_SMTP_PASS"))
		}
	}

	f := strings.TrimSpace(from)
	if f == "" {
		f = strings.TrimSpace(os.Getenv("SMTP_FROM"))
		if f == "" {
			fromEmail := strings.TrimSpace(os.Getenv("APP_SMTP_FROM_EMAIL"))
			fromName := strings.TrimSpace(os.Getenv("APP_SMTP_FROM_NAME"))
			switch {
			case fromEmail != "" && fromName != "":
				f = fmt.Sprintf("%s <%s>", fromName, fromEmail)
			case fromEmail != "":
				f = fromEmail
			}
		}
	}

	// TLS override from env if set
	if raw := strings.TrimSpace(os.Getenv("SMTP_TLS")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			requireTLS = parsed
		}
	} else if raw := strings.TrimSpace(os.Getenv("APP_SMTP_TLS")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			requireTLS = parsed
		}
	}

	if h == "" || p == "" {
		return nil, fmt.Errorf("smtp configuration is incomplete: SMTP_HOST/APP_SMTP_HOST and SMTP_PORT/APP_SMTP_PORT are required")
	}

	portInt, err := strconv.Atoi(p)
	if err != nil || portInt <= 0 || portInt > 65535 {
		return nil, fmt.Errorf("invalid smtp port: %q", p)
	}

	// Timeout (seconds)
	timeout := 20 * time.Second
	if raw := strings.TrimSpace(os.Getenv("APP_SMTP_TIMEOUT_SECONDS")); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	// Redis client (optional)
	var redisClient *redis.Client
	if strings.TrimSpace(redisURL) != "" {
		if opts, err := redis.ParseURL(redisURL); err == nil {
			redisClient = redis.NewClient(opts)
		}
	}

	return &Sender{
		host:       h,
		port:       portInt,
		user:       u,
		pass:       pw,
		from:       f,
		requireTLS: requireTLS,
		timeout:    timeout,
		redis:      redisClient,
	}, nil
}

// SendHTML sends an HTML email.
func (s *Sender) SendHTML(to, subject, body string) error {
	return s.sendInternal(to, subject, body, nil)
}

// SendHTMLWithAttachment sends an HTML email with a single attachment
// (e.g. PDF). contentType can be "" to default to application/pdf.
func (s *Sender) SendHTMLWithAttachment(
	to string,
	subject string,
	body string,
	attachmentName string,
	contentType string,
	payload []byte,
) error {
	if len(payload) == 0 {
		return fmt.Errorf("attachment payload is empty")
	}
	if attachmentName == "" {
		attachmentName = "attachment.pdf"
	}
	if contentType == "" {
		contentType = "application/pdf"
	}

	return s.sendInternal(to, subject, body, func(msg *gomail.Msg) error {
		return msg.AttachReader(
			attachmentName,
			bytes.NewReader(payload),
			gomail.WithFileName(attachmentName),
			gomail.WithFileContentType(gomail.ContentType(contentType)),
		)
	})
}

// sendInternal encapsulates the actual SMTP send logic.
func (s *Sender) sendInternal(
	to string,
	subject string,
	body string,
	mutate func(msg *gomail.Msg) error,
) error {
	if s.host == "" || s.port == 0 {
		return fmt.Errorf("smtp configuration is incomplete")
	}

	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("recipient address is empty")
	}

	// Simple per-recipient rate limiting: 10 mails / minute
	if s.redis != nil {
		ctx := context.Background()
		key := fmt.Sprintf("email_rate:%s", strings.ToLower(to))
		if count, err := s.redis.Incr(ctx, key).Result(); err == nil {
			if count == 1 {
				s.redis.Expire(ctx, key, time.Minute)
			}
			if count > 10 {
				return fmt.Errorf("rate limit exceeded for %s", to)
			}
		}
	}

	headerFrom := strings.TrimSpace(s.from)
	if headerFrom == "" {
		headerFrom = strings.TrimSpace(s.user)
	}
	if headerFrom == "" {
		return fmt.Errorf("smtp from address is missing")
	}

	fromAddr, err := mail.ParseAddress(headerFrom)
	if err != nil {
		return fmt.Errorf("invalid SMTP_FROM address: %w", err)
	}

	// TLS policy
	tlsPolicy := gomail.TLSOpportunistic
	var tlsConfig *tls.Config
	if s.requireTLS {
		tlsPolicy = gomail.TLSMandatory
		tlsConfig = &tls.Config{
			ServerName: s.host,
			MinVersion: tls.VersionTLS12,
		}
	}

	options := []gomail.Option{
		gomail.WithPort(s.port),
		gomail.WithTimeout(s.timeout),
		gomail.WithTLSPolicy(tlsPolicy),
	}

	if tlsConfig != nil {
		options = append(options, gomail.WithTLSConfig(tlsConfig))
	}

	// Implicit SSL for 465 if ever used
	if s.port == 465 {
		options = append(options, gomail.WithSSL())
	}

	// Only configure SMTP AUTH when user+pass are set
	if s.user != "" && s.pass != "" {
		options = append(options,
			gomail.WithUsername(s.user),
			gomail.WithPassword(s.pass),
			// Plain is fine as long as TLS is required for remote providers
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		)
	}

	client, err := gomail.NewClient(s.host, options...)
	if err != nil {
		return fmt.Errorf("failed to initialize smtp client: %w", err)
	}
	defer client.Close()

	msg := gomail.NewMsg()
	if err := msg.From(fromAddr.String()); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("invalid recipient address: %w", err)
	}
	if err := msg.EnvelopeFrom(fromAddr.Address); err != nil {
		return fmt.Errorf("invalid envelope from address: %w", err)
	}

	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, body)

	if mutate != nil {
		if err := mutate(msg); err != nil {
			return fmt.Errorf("failed to apply message mutation: %w", err)
		}
	}

	if err := client.DialAndSend(msg); err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	return nil
}
