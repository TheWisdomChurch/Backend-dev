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

	// timeouts
	dialTimeout   time.Duration
	opTimeout     time.Duration
	writeTimeout  time.Duration
}

func NewSender(redisURL, host, port, user, pass, from string, requireTLS bool) (*Sender, error) {
	if host == "" { host = os.Getenv("SMTP_HOST") }
	if port == "" { port = os.Getenv("SMTP_PORT") }
	if user == "" { user = os.Getenv("SMTP_USER") }
	if pass == "" { pass = os.Getenv("SMTP_PASS") }
	if from == "" { from = os.Getenv("SMTP_FROM") }

	if raw := os.Getenv("SMTP_TLS"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			requireTLS = parsed
		}
	}

	// Optional: override timeouts via env
	dialTimeout := 4 * time.Second   // keep fast; if SMTP is slow, don't block requests
	opTimeout := 6 * time.Second     // hello/starttls/auth/mail/rcpt/data
	writeTimeout := 8 * time.Second  // writing body can be a bit longer

	if v := strings.TrimSpace(os.Getenv("SMTP_DIAL_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil { dialTimeout = d }
	}
	if v := strings.TrimSpace(os.Getenv("SMTP_OP_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil { opTimeout = d }
	}
	if v := strings.TrimSpace(os.Getenv("SMTP_WRITE_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil { writeTimeout = d }
	}

	var redisClient *redis.Client
	if redisURL != "" {
		if opts, err := redis.ParseURL(redisURL); err == nil {
			redisClient = redis.NewClient(opts)
		}
	}

	return &Sender{
		host:        strings.TrimSpace(host),
		port:        strings.TrimSpace(port),
		user:        strings.TrimSpace(user),
		pass:        strings.TrimSpace(pass),
		from:        strings.TrimSpace(from),
		requireTLS:  requireTLS,
		redis:       redisClient,
		dialTimeout: dialTimeout,
		opTimeout:   opTimeout,
		writeTimeout: writeTimeout,
	}, nil
}

func (s *Sender) SendHTML(to, subject, body string) error {
	// Keep old signature; but internally create a short context.
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	return s.SendHTMLContext(ctx, to, subject, body)
}

func (s *Sender) SendHTMLContext(ctx context.Context, to, subject, body string) error {
	// Rate limit: max 10/min per recipient
	if s.redis != nil {
		key := fmt.Sprintf("email_rate:%s:limit", to)
		count, err := s.redis.Incr(ctx, key).Result()
		if err == nil {
			if count == 1 {
				_ = s.redis.Expire(ctx, key, time.Minute).Err()
			}
			if count > 10 {
				return fmt.Errorf("rate limit exceeded for %s", to)
			}
		}
	}

	if s.host == "" || s.port == "" || s.user == "" || s.pass == "" {
		return fmt.Errorf("smtp configuration is incomplete")
	}

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

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := net.JoinHostPort(s.host, s.port)

	tlsConfig := &tls.Config{
		ServerName: s.host,
		MinVersion: tls.VersionTLS12,
	}

	dialer := &net.Dialer{Timeout: s.dialTimeout}

	var (
		conn   net.Conn
		client *smtp.Client
		err    error
	)

	// helper to set deadlines for each “phase”
	setDeadline := func(d time.Duration) {
		_ = conn.SetDeadline(time.Now().Add(d))
	}

	// Connect
	if s.port == "465" {
		// Implicit TLS
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp connect failed: %w", err)
	}
	defer conn.Close()

	setDeadline(s.opTimeout)
	client, err = smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp client failed: %w", err)
	}
	defer func() { _ = client.Close() }()

	// EHLO/HELO (smtp.NewClient already does greeting; explicit Hello is ok)
	setDeadline(s.opTimeout)
	if err := client.Hello("wisdomchurchhq.org"); err != nil {
		return fmt.Errorf("smtp hello failed: %w", err)
	}

	// STARTTLS on 587
	if s.port != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			setDeadline(s.opTimeout)
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("starttls failed: %w", err)
			}
		} else if s.requireTLS {
			return fmt.Errorf("smtp server at %s does not support starttls", addr)
		}
	}

	// AUTH
	setDeadline(s.opTimeout)
	if ok, _ := client.Extension("AUTH"); !ok {
		return fmt.Errorf("smtp server at %s does not advertise AUTH", addr)
	}
	if err := client.Auth(smtp.PlainAuth("", s.user, s.pass, s.host)); err != nil {
		return fmt.Errorf("smtp auth failed: %w", err)
	}

	// MAIL / RCPT
	setDeadline(s.opTimeout)
	if err := client.Mail(envelopeFrom); err != nil {
		return fmt.Errorf("mail from failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to failed: %w", err)
	}

	// DATA
	setDeadline(s.writeTimeout)
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data failed: %w", err)
	}
	if _, err := w.Write([]byte(msg.String())); err != nil {
		_ = w.Close()
		return fmt.Errorf("write failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close writer failed: %w", err)
	}

	// QUIT (best-effort)
	_ = client.Quit()
	return nil
}
