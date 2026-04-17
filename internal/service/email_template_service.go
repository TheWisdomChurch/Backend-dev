package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
)

type EmailTemplateService interface {
	SendTemplateEmail(req *models.SendTemplateEmailRequest) (*models.SendTemplateEmailResponse, error)
}

type emailTemplateService struct {
	sender        EmailSender
	branding      email.Branding
	tplStore      *email.TemplateStore
	remoteEnabled bool
	remoteOnly    bool
	remoteTimeout time.Duration
}

func NewEmailTemplateService(sender EmailSender, branding email.Branding) EmailTemplateService {
	var store *email.TemplateStore
	if strings.TrimSpace(os.Getenv("S3_PUBLIC_BASE_URL")) != "" {
		if ts, err := email.NewTemplateStoreFromEnv(); err == nil {
			store = ts
		}
	}
	remoteEnabled := isTrueEnv("EMAIL_TEMPLATE_REMOTE_ENABLED")
	remoteOnly := isTrueEnv("EMAIL_TEMPLATE_REMOTE_ONLY")
	timeout := 8 * time.Second
	if raw := strings.TrimSpace(os.Getenv("EMAIL_TEMPLATE_REMOTE_TIMEOUT_SECONDS")); raw != "" {
		if sec, err := parseInt(raw); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}
	return &emailTemplateService{
		sender:        sender,
		branding:      branding,
		tplStore:      store,
		remoteEnabled: remoteEnabled || remoteOnly,
		remoteOnly:    remoteOnly,
		remoteTimeout: timeout,
	}
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
	templateKey := string(req.Template)
	heroImageURL := email.TemplateAssetURL(s.branding, templateKey, "hero.png")
	if strings.TrimSpace(req.HeroImageURL) != "" {
		if resolved := email.ResolveTemplateAssetURL(s.branding, req.HeroImageURL); resolved != "" {
			heroImageURL = resolved
		}
	}

	var otpCode string
	var otpExpires time.Time

	switch req.Template {
	case models.EmailTemplateRegistration:
		subject = fmt.Sprintf("Welcome to %s", appName)
	case models.EmailTemplateBirthday:
		subject = fmt.Sprintf("Happy Birthday from %s", appName)
	case models.EmailTemplateOTP:
		otpCode = strings.TrimSpace(req.OTPCode)
		if otpCode == "" {
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
		otpExpires = expires
		subject = fmt.Sprintf("%s verification code", appName)
	default:
		return nil, errors.New("unsupported template")
	}

	if s.remoteEnabled && s.tplStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.remoteTimeout)
		defer cancel()
		remoteBody, err := s.renderRemoteTemplate(ctx, templateKey, req, heroImageURL, otpCode, otpExpires)
		if err == nil && strings.TrimSpace(remoteBody) != "" {
			body = remoteBody
		} else if s.remoteOnly {
			if err == nil {
				err = errors.New("remote template rendered empty body")
			}
			return nil, err
		}
	}

	if strings.TrimSpace(body) == "" {
		switch req.Template {
		case models.EmailTemplateRegistration:
			body = email.RenderRegistrationEmail(email.RegistrationTemplateData{
				Branding:      s.branding,
				RecipientName: req.RecipientName,
				ActionURL:     req.ActionURL,
				Message:       req.CustomMessage,
				HeroImageURL:  heroImageURL,
			})
		case models.EmailTemplateBirthday:
			body = email.RenderBirthdayEmail(email.BirthdayTemplateData{
				Branding:      s.branding,
				RecipientName: req.RecipientName,
				BirthdayDate:  req.BirthdayDate,
				Message:       req.CustomMessage,
				HeroImageURL:  heroImageURL,
			})
		case models.EmailTemplateOTP:
			body = email.RenderOTPEmail(email.OTPTemplateData{
				Branding:     s.branding,
				Code:         otpCode,
				Purpose:      strings.TrimSpace(req.TemplateReason),
				ExpiresAt:    otpExpires,
				ActionURL:    strings.TrimSpace(req.ActionURL),
				HeroImageURL: heroImageURL,
			})
		default:
			return nil, errors.New("unsupported template")
		}
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

func (s *emailTemplateService) renderRemoteTemplate(
	ctx context.Context,
	templateKey string,
	req *models.SendTemplateEmailRequest,
	heroImageURL string,
	otpCode string,
	otpExpires time.Time,
) (string, error) {
	if s.tplStore == nil {
		return "", errors.New("template store not configured")
	}

	branding := s.branding
	if strings.TrimSpace(branding.AppName) == "" {
		branding.AppName = "Wisdom House"
	}

	expiresRFC3339 := ""
	expiresHuman := ""
	if !otpExpires.IsZero() {
		expiresRFC3339 = otpExpires.UTC().Format(time.RFC3339)
		expiresHuman = otpExpires.UTC().Format("Mon, 02 Jan 2006 15:04 MST")
	}

	data := map[string]any{
		"Branding":         branding,
		"Template":         templateKey,
		"AppName":          branding.AppName,
		"LogoURL":          branding.LogoURL,
		"PublicURL":        branding.PublicURL,
		"FrontendURL":      branding.FrontendURL,
		"SupportEmail":     branding.SupportEmail,
		"PastorName":       branding.PastorName,
		"AdminPortalURL":   branding.AdminPortalURL,
		"RecipientName":    strings.TrimSpace(req.RecipientName),
		"Email":            strings.TrimSpace(req.Email),
		"ActionURL":        strings.TrimSpace(req.ActionURL),
		"CustomMessage":    strings.TrimSpace(req.CustomMessage),
		"TemplateReason":   strings.TrimSpace(req.TemplateReason),
		"BirthdayDate":     strings.TrimSpace(req.BirthdayDate),
		"HeroImageURL":     strings.TrimSpace(heroImageURL),
		"OTPCode":          strings.TrimSpace(otpCode),
		"OTPExpiresAt":     expiresRFC3339,
		"OTPExpiresAtText": expiresHuman,
		"Year":             time.Now().UTC().Year(),
	}

	_, htmlOut, _, err := s.tplStore.RenderWithData(ctx, templateKey, data)
	if err != nil {
		return "", err
	}
	return htmlOut, nil
}

func isTrueEnv(key string) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return false
	}
	switch strings.ToLower(val) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseInt(s string) (int, error) {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
