package service

import (
	"encoding/json"
	"fmt"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type SecurityService interface {
	RecordEvent(eventType string, user *models.User, meta LoginMetadata, details map[string]interface{})
	NotifySuspiciousLogin(user *models.User, meta LoginMetadata, reason string)
}

type securityServiceImpl struct {
	eventsRepo  repository.SecurityEventRepository
	devRepo     repository.TrustedDeviceRepository
	sender      EmailSender
	branding    email.Branding
	frontendURL string
}

func NewSecurityService(
	eventsRepo repository.SecurityEventRepository,
	devRepo repository.TrustedDeviceRepository,
	sender EmailSender,
	branding email.Branding,
	frontendURL string,
) SecurityService {
	return &securityServiceImpl{
		eventsRepo:  eventsRepo,
		devRepo:     devRepo,
		sender:      sender,
		branding:    branding,
		frontendURL: frontendURL,
	}
}

func (s *securityServiceImpl) RecordEvent(eventType string, user *models.User, meta LoginMetadata, details map[string]interface{}) {
	if s.eventsRepo == nil {
		return
	}
	var metaJSON []byte
	if len(details) > 0 {
		metaJSON, _ = json.Marshal(details)
	}

	ev := &models.SecurityEvent{
		Type:      eventType,
		Email:     safeEmail(user),
		UserID:    safeUserID(user),
		IP:        meta.IP,
		UserAgent: meta.UserAgent,
		CreatedAt: time.Now().UTC(),
	}
	if len(metaJSON) > 0 {
		ev.Metadata = metaJSON
	}
	_ = s.eventsRepo.Create(ev)
}

func (s *securityServiceImpl) NotifySuspiciousLogin(user *models.User, meta LoginMetadata, reason string) {
	if s.sender == nil || user == nil {
		return
	}
	s.RecordEvent("suspicious_login", user, meta, map[string]interface{}{"reason": reason})

	body := email.RenderSecurityAlertEmail(email.SecurityAlertTemplateData{
		Branding:  s.branding,
		Email:     user.Email,
		Reason:    reason,
		IP:        meta.IP,
		UserAgent: meta.UserAgent,
		Timestamp: time.Now().UTC().Format(time.RFC1123),
		ManageURL: fmt.Sprintf("%s/security/devices", s.frontendURL),
	})

	subject := fmt.Sprintf("[%s] Verify a recent sign-in", s.branding.AppName)
	_ = s.sender.SendHTML(user.Email, subject, body)
}

func safeEmail(u *models.User) string {
	if u == nil {
		return ""
	}
	return u.Email
}

func safeUserID(u *models.User) *string {
	if u == nil {
		return nil
	}
	id := u.ID
	return &id
}
