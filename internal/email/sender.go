package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

type Sender struct {
    host   string
    port   string
    user   string
    pass   string
    from   string
    redis  *redis.Client // For rate limiting
}

func NewSender(redisURL string) (*Sender, error) {
    // Create Redis client for rate limiting
    var redisClient *redis.Client
    if redisURL != "" {
        opts, err := redis.ParseURL(redisURL)
        if err == nil {
            redisClient = redis.NewClient(opts)
        }
    }
    
    return &Sender{
        host:  os.Getenv("SMTP_HOST"),
        port:  os.Getenv("SMTP_PORT"),
        user:  os.Getenv("SMTP_USER"),
        pass:  os.Getenv("SMTP_PASS"),
        from:  os.Getenv("SMTP_FROM"),
        redis: redisClient,
    }, nil
}

// SendHTML sends HTML email with rate limiting
func (s *Sender) SendHTML(to, subject, body string) error {
    // Rate limiting: max 10 emails per minute per recipient
    if s.redis != nil {
        key := fmt.Sprintf("email_rate:%s", to)
        limitKey := fmt.Sprintf("%s:limit", key)
        
        // Check rate limit
        count, err := s.redis.Incr(context.Background(), limitKey).Result()
        if err == nil {
            if count == 1 {
                s.redis.Expire(context.Background(), limitKey, time.Minute)
            }
            if count > 10 {
                return fmt.Errorf("rate limit exceeded for %s", to)
            }
        }
    }
    
    // Prepare message
    headers := make(map[string]string)
    headers["From"] = s.from
    headers["To"] = to
    headers["Subject"] = subject
    headers["MIME-Version"] = "1.0"
    headers["Content-Type"] = "text/html; charset=UTF-8"
    
    var message strings.Builder
    for key, value := range headers {
        message.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
    }
    message.WriteString("\r\n" + body)
    
    // TLS configuration
    tlsConfig := &tls.Config{
        ServerName: s.host,
    }
    
    // Connect with TLS
    conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%s", s.host, s.port), tlsConfig)
    if err != nil {
        return fmt.Errorf("TLS connection failed: %w", err)
    }
    defer conn.Close()
    
    client, err := smtp.NewClient(conn, s.host)
    if err != nil {
        return fmt.Errorf("SMTP client failed: %w", err)
    }
    defer client.Close()
    
    // Authenticate
    auth := smtp.PlainAuth("", s.user, s.pass, s.host)
    if err := client.Auth(auth); err != nil {
        return fmt.Errorf("authentication failed: %w", err)
    }
    
    // Send email
    if err := client.Mail(s.user); err != nil {
        return fmt.Errorf("MAIL command failed: %w", err)
    }
    if err := client.Rcpt(to); err != nil {
        return fmt.Errorf("RCPT command failed: %w", err)
    }
    
    w, err := client.Data()
    if err != nil {
        return fmt.Errorf("DATA command failed: %w", err)
    }
    
    _, err = w.Write([]byte(message.String()))
    if err != nil {
        return fmt.Errorf("writing message failed: %w", err)
    }
    
    err = w.Close()
    if err != nil {
        return fmt.Errorf("closing writer failed: %w", err)
    }
    
    return nil
}