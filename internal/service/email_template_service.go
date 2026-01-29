package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
)

type EmailTemplateService interface {
	SendTemplateEmail(req *models.SendTemplateEmailRequest) (*models.SendTemplateEmailResponse, error)
}

type emailTemplateService struct {
	sender   EmailSender
	branding email.Branding
}

func NewEmailTemplateService(sender EmailSender, branding email.Branding) EmailTemplateService {
	return &emailTemplateService{sender: sender, branding: branding}
}

func (s *emailTemplateService) SendTemplateEmail(req *models.SendTemplateEmailRequest) (*models.SendTemplateEmailResponse, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}

	to := strings.TrimSpace(req.Email)
	if to == "" {
		return nil, errors.New("email is required")
	}

	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}

	var subject string
	var body string

	switch req.Template {
	case models.EmailTemplateRegistration:
		subject = fmt.Sprintf("Welcome to %s", appName)
		body = email.RenderRegistrationEmail(email.RegistrationTemplateData{
			Branding:      s.branding,
			RecipientName: req.RecipientName,
			ActionURL:     req.ActionURL,
			Message:       req.CustomMessage,
			HeroImageURL:  email.TemplateAssetURL(s.branding, "registration", "hero.png"),
		})
	case models.EmailTemplateBirthday:
		subject = fmt.Sprintf("Happy Birthday from %s", appName)
		body = email.RenderBirthdayEmail(email.BirthdayTemplateData{
			Branding:      s.branding,
			RecipientName: req.RecipientName,
			BirthdayDate:  req.BirthdayDate,
			Message:       req.CustomMessage,
			HeroImageURL:  email.TemplateAssetURL(s.branding, "birthday", "hero.png"),
		})
	case models.EmailTemplateOTP:
		code := strings.TrimSpace(req.OTPCode)
		if code == "" {
			return nil, errors.New("otpCode is required for otp template")
		}
		expires := time.Now().UTC().Add(10 * time.Minute)
		if strings.TrimSpace(req.OTPExpiresAt) != "" {
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.OTPExpiresAt))
			if err != nil {
				return nil, errors.New("otpExpiresAt must be RFC3339")
			}
			expires = parsed
		}
		subject = fmt.Sprintf("%s verification code", appName)
		body = email.RenderOTPEmail(email.OTPTemplateData{
			Branding:     s.branding,
			Code:         code,
			Purpose:      strings.TrimSpace(req.TemplateReason),
			ExpiresAt:    expires,
			ActionURL:    strings.TrimSpace(req.ActionURL),
			HeroImageURL: email.TemplateAssetURL(s.branding, "otp", "hero.png"),
		})
	default:
		return nil, errors.New("unsupported template")
	}

	if err := s.sender.SendHTML(to, subject, body); err != nil {
		return nil, err
	}

	return &models.SendTemplateEmailResponse{
		Email:    to,
		Template: string(req.Template),
		SentAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}
