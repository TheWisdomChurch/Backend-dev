package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

type Sender struct {
	host       string
	port       string
	user       string
	pass       string
	from       string
	requireTLS bool
	redis      *redis.Client
}

func NewSender(redisURL, host, port, user, pass, from string, requireTLS bool) (*Sender, error) {
	// Fallback to envs if args empty
	if host == "" {
		host = os.Getenv("SMTP_HOST")
	}
	if port == "" {
		port = os.Getenv("SMTP_PORT")
	}
	if user == "" {
		user = os.Getenv("SMTP_USER")
	}
	if pass == "" {
		pass = os.Getenv("SMTP_PASS")
	}
	if from == "" {
		from = os.Getenv("SMTP_FROM")
	}
	if raw := os.Getenv("SMTP_TLS"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			requireTLS = parsed
		}
	}

	// Redis for rate limiting
	var redisClient *redis.Client
	if redisURL != "" {
		if opts, err := redis.ParseURL(redisURL); err == nil {
			redisClient = redis.NewClient(opts)
		}
	}

	return &Sender{
		host:       strings.TrimSpace(host),
		port:       strings.TrimSpace(port),
		user:       strings.TrimSpace(user),
		pass:       strings.TrimSpace(pass),
		from:       strings.TrimSpace(from),
		requireTLS: requireTLS,
		redis:      redisClient,
	}, nil
}

// SendHTML sends HTML email with rate limiting
func (s *Sender) SendHTML(to, subject, body string) error {
	// Rate limiting: max 10 emails per minute per recipient
	if s.redis != nil {
		key := fmt.Sprintf("email_rate:%s", to)
		limitKey := fmt.Sprintf("%s:limit", key)
		if count, err := s.redis.Incr(context.Background(), limitKey).Result(); err == nil {
			if count == 1 {
				s.redis.Expire(context.Background(), limitKey, time.Minute)
			}
			if count > 10 {
				return fmt.Errorf("rate limit exceeded for %s", to)
			}
		}
	}

	if s.host == "" || s.port == "" || s.user == "" || s.pass == "" {
		return fmt.Errorf("smtp configuration is incomplete")
	}

	// Header From can be pretty; envelope MAIL FROM must match auth user for Gmail
	headerFrom := strings.TrimSpace(s.from)
	if headerFrom == "" {
		headerFrom = s.user
	}
	envelopeFrom := s.user

	headers := map[string]string{
		"From":         headerFrom,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	var message strings.Builder
	for k, v := range headers {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n")
	message.WriteString(body)

	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	tlsConfig := &tls.Config{
		ServerName: s.host,
		MinVersion: tls.VersionTLS12,
	}

	helo := "wisdomhouse.app"

	var client *smtp.Client

	if s.port == "465" {
		// Implicit TLS
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("TLS connection failed: %w", err)
		}
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

		c, err := smtp.NewClient(conn, s.host)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("SMTP client failed: %w", err)
		}
		client = c
	} else {
		// Plain + STARTTLS (Gmail 587)
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("SMTP connection failed: %w", err)
		}
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

		c, err := smtp.NewClient(conn, s.host)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("SMTP client failed: %w", err)
		}

		// HELO/EHLO
		if err := c.Hello(helo); err != nil {
			_ = c.Close()
			return fmt.Errorf("SMTP hello failed: %w", err)
		}

		startTLSSupported := false
		if ok, _ := c.Extension("STARTTLS"); ok {
			startTLSSupported = true
			if err := c.StartTLS(tlsConfig); err != nil {
				_ = c.Close()
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
			// Re-HELO after TLS
			if err := c.Hello(helo); err != nil {
				_ = c.Close()
				return fmt.Errorf("SMTP hello after STARTTLS failed: %w", err)
			}
		}

		if s.requireTLS && !startTLSSupported {
			_ = c.Close()
			return fmt.Errorf("SMTP server at %s does not support STARTTLS", addr)
		}

		client = c
	}

	defer func() { _ = client.Quit() }()

	// AUTH
	if ok, _ := client.Extension("AUTH"); !ok {
		_ = client.Close()
		return fmt.Errorf("SMTP server at %s does not advertise AUTH", addr)
	}

	if err := client.Auth(smtp.PlainAuth("", s.user, s.pass, s.host)); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Envelope + recipient
	if err := client.Mail(envelopeFrom); err != nil {
		return fmt.Errorf("MAIL command failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT command failed: %w", err)
	}

	// DATA
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}
	if _, err := w.Write([]byte(message.String())); err != nil {
		return fmt.Errorf("writing message failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing writer failed: %w", err)
	}

	return nil
}
