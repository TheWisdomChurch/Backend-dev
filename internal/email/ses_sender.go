package email

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/go-redis/redis/v8"
)

// SESSender sends email via AWS SES.
type SESSender struct {
	client    *ses.Client
	from      string
	redis     *redis.Client
	opTimeout time.Duration
}

func NewSESSender(redisURL, region, from string) (*SESSender, error) {
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if from == "" {
		from = os.Getenv("SES_FROM_EMAIL")
	}

	region = strings.TrimSpace(region)
	from = strings.TrimSpace(from)

	if region == "" {
		return nil, fmt.Errorf("aws region is required for SES")
	}
	if from == "" {
		return nil, fmt.Errorf("ses from email is required")
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("aws config load failed: %w", err)
	}

	var redisClient *redis.Client
	if redisURL != "" {
		if opts, err := redis.ParseURL(redisURL); err == nil {
			redisClient = redis.NewClient(opts)
		}
	}

	return &SESSender{
		client:    ses.NewFromConfig(cfg),
		from:      from,
		redis:     redisClient,
		opTimeout: 10 * time.Second,
	}, nil
}

func (s *SESSender) SendHTML(to, subject, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.opTimeout)
	defer cancel()
	return s.SendHTMLContext(ctx, to, subject, body)
}

func (s *SESSender) SendHTMLContext(ctx context.Context, to, subject, body string) error {
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

	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("recipient is required")
	}
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("subject is required")
	}

	input := &ses.SendEmailInput{
		Source: aws.String(s.from),
		Destination: &types.Destination{
			ToAddresses: []string{to},
		},
		Message: &types.Message{
			Subject: &types.Content{
				Data:    aws.String(subject),
				Charset: aws.String("UTF-8"),
			},
			Body: &types.Body{
				Html: &types.Content{
					Data:    aws.String(body),
					Charset: aws.String("UTF-8"),
				},
			},
		},
	}

	if _, err := s.client.SendEmail(ctx, input); err != nil {
		return fmt.Errorf("ses send failed: %w", err)
	}

	return nil
}
