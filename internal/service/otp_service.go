package service

import (
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

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

const otpLength = 6

var otpTTL = 10 * time.Minute

type OTPService interface {
	SendOTP(req *models.SendOTPRequest) (*models.SendOTPResponse, error)
	VerifyOTP(req *models.VerifyOTPRequest) (*models.VerifyOTPResponse, error)
}

type otpService struct {
	repo     *repository.OTPRepository
	sender   EmailSender
	branding email.Branding
}

func NewOTPService(repo *repository.OTPRepository, sender EmailSender, branding email.Branding) OTPService {
	return &otpService{repo: repo, sender: sender, branding: branding}
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
		Branding:    s.branding,
		Code:        code,
		Purpose:     purpose,
		ExpiresAt:   expiresAt,
		ActionURL:   strings.TrimSpace(req.ActionURL),
		ActionLabel: strings.TrimSpace(req.ActionLabel),
	})

	if err := s.sender.SendHTML(emailAddr, "Your Wisdom House verification code", body); err != nil {
		return nil, err
	}

	return &models.SendOTPResponse{
		ExpiresAt: expiresAt,
		Purpose:   purpose,
		ActionURL: strings.TrimSpace(req.ActionURL),
	}, nil
}

func (s *otpService) VerifyOTP(req *models.VerifyOTPRequest) (*models.VerifyOTPResponse, error) {
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
			return nil, errors.New("otp not found or expired")
		}
		return nil, err
	}

	candidate := hashOTP(otp.CodeSalt, code)
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(otp.CodeHash)) != 1 {
		return nil, errors.New("invalid code")
	}

	usedAt := time.Now().UTC()
	if err := s.repo.MarkUsed(otp.ID, usedAt); err != nil {
		return nil, err
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
