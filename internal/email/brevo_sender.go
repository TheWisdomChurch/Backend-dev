package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const defaultBrevoBaseURL = "https://api.brevo.com"

// BrevoSender sends transactional email via Brevo API.
type BrevoSender struct {
	apiKey    string
	fromEmail string
	fromName  string
	baseURL   string
	client    *http.Client
	redis     *redis.Client
	opTimeout time.Duration
}

func NewBrevoSender(redisURL, apiKey, fromEmail, fromName, baseURL string) (*BrevoSender, error) {
	if apiKey == "" {
		apiKey = os.Getenv("BREVO_API_KEY")
	}
	if fromEmail == "" {
		fromEmail = os.Getenv("BREVO_FROM_EMAIL")
	}
	if fromName == "" {
		fromName = os.Getenv("BREVO_FROM_NAME")
	}
	if baseURL == "" {
		baseURL = os.Getenv("BREVO_BASE_URL")
	}

	apiKey = strings.TrimSpace(apiKey)
	fromEmail = strings.TrimSpace(fromEmail)
	fromName = strings.TrimSpace(fromName)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBrevoBaseURL
	}

	if apiKey == "" {
		return nil, fmt.Errorf("brevo api key is required")
	}
	if fromEmail == "" {
		return nil, fmt.Errorf("brevo from email is required")
	}

	var redisClient *redis.Client
	if redisURL != "" {
		if opts, err := redis.ParseURL(redisURL); err == nil {
			redisClient = redis.NewClient(opts)
		}
	}

	opTimeout := 12 * time.Second

	return &BrevoSender{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		fromName:  fromName,
		baseURL:   baseURL,
		client:    &http.Client{Timeout: opTimeout},
		redis:     redisClient,
		opTimeout: opTimeout,
	}, nil
}

func (s *BrevoSender) SendHTML(to, subject, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.opTimeout)
	defer cancel()
	return s.SendHTMLTextContext(ctx, to, subject, body, "")
}

func (s *BrevoSender) SendHTMLText(to, subject, htmlBody, textBody string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.opTimeout)
	defer cancel()
	return s.SendHTMLTextContext(ctx, to, subject, htmlBody, textBody)
}

func (s *BrevoSender) SendHTMLContext(ctx context.Context, to, subject, body string) error {
	return s.SendHTMLTextContext(ctx, to, subject, body, "")
}

func (s *BrevoSender) SendHTMLTextContext(ctx context.Context, to, subject, htmlBody, textBody string) error {
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

	to = strings.TrimSpace(to)
	subject = strings.TrimSpace(subject)
	if to == "" {
		return fmt.Errorf("recipient is required")
	}
	if subject == "" {
		return fmt.Errorf("subject is required")
	}
	if strings.TrimSpace(htmlBody) == "" {
		return fmt.Errorf("html body is required")
	}

	type recipient struct {
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
	}
	type sender struct {
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
	}
	type brevoRequest struct {
		Sender      sender      `json:"sender"`
		To          []recipient `json:"to"`
		Subject     string      `json:"subject"`
		TextContent string      `json:"textContent,omitempty"`
		HTMLContent string      `json:"htmlContent"`
	}

	payload := brevoRequest{
		Sender: sender{
			Email: s.fromEmail,
			Name:  s.fromName,
		},
		To:          []recipient{{Email: to}},
		Subject:     subject,
		TextContent: strings.TrimSpace(textBody),
		HTMLContent: htmlBody,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("brevo payload marshal failed: %w", err)
	}

	endpoint := s.baseURL + "/v3/smtp/email"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("brevo request create failed: %w", err)
	}
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("brevo send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("brevo send failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	return nil
}
