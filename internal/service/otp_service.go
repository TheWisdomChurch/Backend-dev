package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

const otpLength = 6

var otpTTL = 10 * time.Minute

// OTP rate-limit constants.
const (
	otpSendMaxPerWindow   = 5               // max OTP sends per email per window
	otpVerifyMaxFails     = 5               // max failed verifications before block
	otpRateLimitWindow    = 10 * time.Minute
)

type OTPService interface {
	SendOTP(req *models.SendOTPRequest) (*models.SendOTPResponse, error)
	VerifyOTP(req *models.VerifyOTPRequest) (*models.VerifyOTPResponse, error)
	VerifyOTPNoConsume(req *models.VerifyOTPRequest) (*models.VerifyOTPResponse, error)
}

type otpService struct {
	repo       *repository.OTPRepository
	userRepo   repository.UserRepository
	sender     EmailSender
	branding   email.Branding
	redisCache *cache.RedisClient // nil when Redis is not configured
}

func NewOTPService(repo *repository.OTPRepository, sender EmailSender, branding email.Branding, userRepo repository.UserRepository, redisCache *cache.RedisClient) OTPService {
	return &otpService{repo: repo, sender: sender, branding: branding, userRepo: userRepo, redisCache: redisCache}
}

// otpSendRateLimitKey returns the Redis key used to track OTP send attempts.
func otpSendRateLimitKey(email string) string {
	return "otp:send:" + email
}

// otpVerifyFailKey returns the Redis key used to track failed OTP verifications.
func otpVerifyFailKey(email, purpose string) string {
	return "otp:fail:" + email + ":" + purpose
}

// checkOTPSendLimit returns an error if the send rate limit is exceeded.
func (s *otpService) checkOTPSendLimit(emailAddr string) error {
	if s.redisCache == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	allowed, err := s.redisCache.RateLimit(ctx, otpSendRateLimitKey(emailAddr), otpSendMaxPerWindow, otpRateLimitWindow)
	if err != nil {
		// Fail open — don't block users if Redis is down
		return nil
	}
	if !allowed {
		return errors.New("too many OTP requests, please wait before requesting again")
	}
	return nil
}

// recordOTPVerifyFail increments the failure counter; returns true when limit exceeded.
func (s *otpService) recordOTPVerifyFail(emailAddr, purpose string) bool {
	if s.redisCache == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := otpVerifyFailKey(emailAddr, purpose)
	// RateLimit call: check if we've exceeded otpVerifyMaxFails within the window.
	allowed, err := s.redisCache.RateLimit(ctx, key, otpVerifyMaxFails, otpRateLimitWindow)
	if err != nil {
		return false
	}
	return !allowed
}

// resetOTPVerifyFails clears the failure counter after a successful verification.
func (s *otpService) resetOTPVerifyFails(emailAddr, purpose string) {
	if s.redisCache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.redisCache.Delete(ctx, otpVerifyFailKey(emailAddr, purpose))
}

func (s *otpService) SendOTP(req *models.SendOTPRequest) (*models.SendOTPResponse, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}

	emailAddr := normalizeEmail(req.Email)
	if emailAddr == "" {
		return nil, errors.New("email is required")
	}

	purpose := strings.TrimSpace(req.Purpose)

	// Enforce send rate limit before any DB work.
	if err := s.checkOTPSendLimit(emailAddr); err != nil {
		return nil, err
	}

	// Only registered & active users should receive login / password reset OTP
	if s.userRepo != nil && (strings.HasPrefix(purpose, "login") || strings.HasPrefix(purpose, "password_reset")) {
		user, err := s.userRepo.FindByEmail(emailAddr)
		if err != nil || user == nil {
			return nil, errors.New("account not found")
		}
		if !user.IsActive {
			return nil, errors.New("account is deactivated")
		}
	}

	code, err := generateOTPCode()
	if err != nil {
		return nil, errors.New("failed to generate otp")
	}

	salt, err := generateSalt()
	if err != nil {
		return nil, errors.New("failed to generate otp")
	}

	hash := hashOTP(salt, code)
	expiresAt := time.Now().UTC().Add(otpTTL)

	_ = s.repo.InvalidateActive(emailAddr, purpose, time.Now().UTC())

	otp := &models.OTP{
		Email:     emailAddr,
		Purpose:   purpose,
		CodeHash:  hash,
		CodeSalt:  salt,
		ExpiresAt: expiresAt,
	}

	if err := s.repo.Create(otp); err != nil {
		return nil, err
	}

	body := email.RenderOTPEmail(email.OTPTemplateData{
		Branding:     s.branding,
		Code:         code,
		Purpose:      purpose,
		ExpiresAt:    expiresAt,
		ActionURL:    strings.TrimSpace(req.ActionURL),
		ActionLabel:  strings.TrimSpace(req.ActionLabel),
		HeroImageURL: email.TemplateAssetURL(s.branding, "otp", "hero.png"),
	})

	subject := otpSubject(s.branding.AppName, purpose)
	if err := s.sender.SendHTML(emailAddr, subject, body); err != nil {
		return nil, err
	}

	return &models.SendOTPResponse{
		ExpiresAt: expiresAt,
		Purpose:   purpose,
		ActionURL: strings.TrimSpace(req.ActionURL),
	}, nil
}

func otpSubject(appName, purpose string) string {
	name := strings.TrimSpace(appName)
	if name == "" {
		name = "Wisdom House"
	}

	p := strings.TrimSpace(purpose)
	if i := strings.Index(p, ":"); i >= 0 {
		p = p[:i]
	}

	switch p {
	case "password_reset":
		return fmt.Sprintf("Reset your %s password", name)
	case "login":
		return fmt.Sprintf("Approve sign-in to %s", name)
	default:
		return fmt.Sprintf("Your %s verification code", name)
	}
}

func (s *otpService) VerifyOTP(req *models.VerifyOTPRequest) (*models.VerifyOTPResponse, error) {
	return s.verifyOTP(req, true)
}

// VerifyOTPNoConsume validates an OTP without marking it as used.
// Useful for multi-step flows (e.g. password reset) where a second call
// will consume the OTP during the final action.
func (s *otpService) VerifyOTPNoConsume(req *models.VerifyOTPRequest) (*models.VerifyOTPResponse, error) {
	return s.verifyOTP(req, false)
}

func (s *otpService) verifyOTP(req *models.VerifyOTPRequest, consume bool) (*models.VerifyOTPResponse, error) {
	emailAddr := normalizeEmail(req.Email)
	if emailAddr == "" {
		return nil, errors.New("email is required")
	}

	code := strings.TrimSpace(req.Code)
	if len(code) != otpLength {
		return nil, errors.New("invalid code")
	}

	purpose := strings.TrimSpace(req.Purpose)

	otp, err := s.repo.GetActive(emailAddr, purpose)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If purpose is a prefix (e.g. "password_reset"), fall back to latest active OTP
			// whose purpose starts with that prefix (e.g. "password_reset:xyz").
			isPrefix := false
			prefix := purpose
			if prefix != "" {
				if strings.HasSuffix(prefix, ":") {
					isPrefix = true
				} else if !strings.Contains(prefix, ":") {
					isPrefix = true
					prefix = prefix + ":"
				}
			}
			if isPrefix {
				if alt, err2 := s.repo.GetLatestActiveByPrefix(emailAddr, prefix); err2 == nil {
					otp = alt
				} else if errors.Is(err2, gorm.ErrRecordNotFound) {
					return nil, errors.New("otp not found or expired")
				} else {
					return nil, err2
				}
			} else {
				return nil, errors.New("otp not found or expired")
			}
		} else {
			return nil, err
		}
	}

	// Check whether this email+purpose has exceeded the failure limit.
	if s.recordOTPVerifyFail(emailAddr, purpose) {
		return nil, errors.New("too many incorrect attempts, please request a new code")
	}

	candidate := hashOTP(otp.CodeSalt, code)
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(otp.CodeHash)) != 1 {
		return nil, errors.New("invalid code")
	}

	// Successful verification — clear the failure counter.
	s.resetOTPVerifyFails(emailAddr, purpose)

	if consume {
		usedAt := time.Now().UTC()
		if err := s.repo.MarkUsed(otp.ID, usedAt); err != nil {
			return nil, err
		}
	}

	return &models.VerifyOTPResponse{Verified: true}, nil
}

func generateOTPCode() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func generateSalt() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashOTP(salt, code string) string {
	sum := sha256.Sum256([]byte(salt + code))
	return hex.EncodeToString(sum[:])
}
