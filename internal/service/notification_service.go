package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type EmailSender interface {
	SendHTML(to, subject, body string) error
}

type NotificationService interface {
	Subscribe(req *models.SubscribeRequest) (*models.Subscriber, error)
	Unsubscribe(email string) error
	UnsubscribeByToken(token string) error
	ListSubscribers(page, limit int, status, search, source string) ([]models.Subscriber, int64, error)
	GetSubscriberSummary() (*models.SubscriberSummary, error)
	SendNotification(req *models.SendNotificationRequest) (*models.SendNotificationResult, error)
}

type notificationService struct {
	subscribers   *repository.SubscriberRepository
	notifications *repository.NotificationRepository
	eventRepo     *repository.EventRepository
	sender        EmailSender
	branding      email.Branding
	protector     *authutil.Protector
}

func NewNotificationService(
	subscriberRepo *repository.SubscriberRepository,
	notificationRepo *repository.NotificationRepository,
	eventRepo *repository.EventRepository,
	sender EmailSender,
	branding email.Branding,
	authSecret string,
) (NotificationService, error) {
	if subscriberRepo == nil {
		return nil, errors.New("subscriber repository is required")
	}
	protector, err := authutil.NewProtector(authSecret)
	if err != nil {
		return nil, fmt.Errorf("configure subscription tokens: %w", err)
	}
	return &notificationService{
		subscribers:   subscriberRepo,
		notifications: notificationRepo,
		eventRepo:     eventRepo,
		sender:        sender,
		branding:      branding,
		protector:     protector,
	}, nil
}

func (s *notificationService) UnsubscribeByToken(token string) error {
	plaintext, err := s.protector.DecryptString(strings.TrimSpace(token))
	if err != nil {
		return errors.New("unsubscribe link is invalid")
	}
	purpose, emailAddr, ok := strings.Cut(plaintext, "\n")
	if !ok || purpose != "unsubscribe" || normalizeEmail(emailAddr) == "" {
		return errors.New("unsubscribe link is invalid")
	}
	return s.Unsubscribe(emailAddr)
}

func (s *notificationService) unsubscribeURL(emailAddr string) string {
	base := strings.TrimRight(strings.TrimSpace(s.branding.PublicURL), "/")
	if base == "" || s.protector == nil {
		return ""
	}
	token, err := s.protector.EncryptString("unsubscribe\n" + normalizeEmail(emailAddr))
	if err != nil {
		return ""
	}
	return base + "/api/v1/notifications/unsubscribe?token=" + url.QueryEscape(token)
}

func (s *notificationService) Subscribe(req *models.SubscribeRequest) (*models.Subscriber, error) {
	emailAddr := normalizeEmail(req.Email)
	if emailAddr == "" {
		return nil, errors.New("email is required")
	}

	existing, err := s.subscribers.GetByEmail(emailAddr)
	if err == nil {
		wasInactive := existing.Status != models.SubscriberStatusActive
		existing.Status = models.SubscriberStatusActive
		if req.Name != nil {
			existing.Name = req.Name
		}
		if req.Source != nil {
			existing.Source = req.Source
		}
		existing.UnsubscribedAt = nil
		if err := s.subscribers.Update(existing); err != nil {
			return nil, err
		}
		if wasInactive {
			s.sendSubscriberWelcome(existing)
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	sub := &models.Subscriber{
		Email:  emailAddr,
		Name:   req.Name,
		Source: req.Source,
		Status: models.SubscriberStatusActive,
	}
	if err := s.subscribers.Create(sub); err != nil {
		return nil, err
	}
	s.sendSubscriberWelcome(sub)
	return sub, nil
}

func (s *notificationService) Unsubscribe(email string) error {
	emailAddr := normalizeEmail(email)
	if emailAddr == "" {
		return errors.New("email is required")
	}

	sub, err := s.subscribers.GetByEmail(emailAddr)
	if err != nil {
		return err
	}

	sub.Status = models.SubscriberStatusUnsubscribed
	now := time.Now().UTC()
	sub.UnsubscribedAt = &now
	return s.subscribers.Update(sub)
}

func (s *notificationService) ListSubscribers(page, limit int, status, search, source string) ([]models.Subscriber, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.subscribers.List(offset, limit, status, search, source)
}

func (s *notificationService) GetSubscriberSummary() (*models.SubscriberSummary, error) {
	return s.subscribers.GetSummary()
}

func (s *notificationService) SendNotification(req *models.SendNotificationRequest) (*models.SendNotificationResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}

	nType := normalizeNotificationType(req.Type)
	if nType == "" {
		return nil, errors.New("type must be 'update' or 'event'")
	}

	audience := "newsletter_subscribers"
	if req.Audience != nil && strings.TrimSpace(*req.Audience) != "" {
		audience = strings.ToLower(strings.TrimSpace(*req.Audience))
	}
	if audience != "newsletter_subscribers" {
		return nil, errors.New("audience must be newsletter_subscribers")
	}

	if containsSensitiveNotificationContent(req.Subject, req.Title, req.Message) {
		return nil, errors.New("message appears to contain internal-only content; use admin inbox for operational alerts")
	}

	if nType == string(models.NotificationTypeEvent) && req.EventID == nil {
		return nil, errors.New("eventId is required for event notifications")
	}

	var event *models.Event
	if req.EventID != nil {
		if s.eventRepo == nil {
			return nil, errors.New("event repository is not configured")
		}
		found, err := s.eventRepo.GetByID(*req.EventID)
		if err != nil {
			return nil, fmt.Errorf("event not found")
		}
		event = found
	}

	notification := &models.Notification{
		Type:    models.NotificationType(nType),
		Subject: strings.TrimSpace(req.Subject),
		Title:   strings.TrimSpace(req.Title),
		Message: strings.TrimSpace(req.Message),
		EventID: req.EventID,
	}

	if notification.Subject == "" || notification.Title == "" || notification.Message == "" {
		return nil, errors.New("subject, title, and message are required")
	}

	subscribers, err := s.subscribers.ListActive()
	if err != nil {
		return nil, err
	}
	if len(subscribers) == 0 {
		return nil, errors.New("no active subscribers")
	}

	if err := s.notifications.Create(notification); err != nil {
		return nil, err
	}

	result := &models.SendNotificationResult{
		Notification: notification,
		Total:        len(subscribers),
	}

	for _, sub := range subscribers {
		unsubscribeURL := s.unsubscribeURL(sub.Email)
		delivery := &models.NotificationDelivery{
			NotificationID: notification.ID,
			SubscriberID:   sub.ID,
			Status:         models.DeliveryStatusPending,
		}
		if err := s.notifications.CreateDelivery(delivery); err != nil {
			result.Failed++
			continue
		}

		body := email.RenderNotificationEmail(email.NotificationTemplateData{
			Branding:       s.branding,
			Title:          notification.Title,
			Message:        notification.Message,
			Event:          event,
			RecipientName:  sub.Name,
			UnsubscribeURL: unsubscribeURL,
		})
		textBody := email.RenderNotificationText(email.NotificationTemplateData{
			Title: notification.Title, Message: notification.Message, Event: event,
			RecipientName: sub.Name, UnsubscribeURL: unsubscribeURL,
		})

		var sendErr error
		if sender, ok := s.sender.(interface {
			SendHTMLTextWithOptions(string, string, string, string, email.MessageOptions) error
		}); ok {
			sendErr = sender.SendHTMLTextWithOptions(sub.Email, notification.Subject, body, textBody, email.MessageOptions{UnsubscribeURL: unsubscribeURL})
		} else if sender, ok := s.sender.(interface {
			SendHTMLText(string, string, string, string) error
		}); ok {
			sendErr = sender.SendHTMLText(sub.Email, notification.Subject, body, textBody)
		} else {
			sendErr = s.sender.SendHTML(sub.Email, notification.Subject, body)
		}
		if sendErr != nil {
			errMsg := sendErr.Error()
			delivery.Status = models.DeliveryStatusFailed
			delivery.ErrorMessage = &errMsg
			_ = s.notifications.UpdateDelivery(delivery)
			result.Failed++
			continue
		}

		sentAt := time.Now().UTC()
		delivery.Status = models.DeliveryStatusSent
		delivery.SentAt = &sentAt
		if err := s.notifications.UpdateDelivery(delivery); err != nil {
			result.Failed++
			continue
		}
		sub.LastNotifiedAt = &sentAt
		_ = s.subscribers.Update(&sub)
		result.Sent++
	}

	return result, nil
}

func (s *notificationService) sendSubscriberWelcome(sub *models.Subscriber) {
	if s.sender == nil {
		return
	}
	unsubscribeURL := s.unsubscribeURL(sub.Email)
	name := ""
	if sub.Name != nil {
		name = *sub.Name
	}
	body := email.RenderSubscriberWelcomeEmail(email.SubscriberWelcomeTemplateData{
		Branding:       s.branding,
		RecipientName:  name,
		UnsubscribeURL: unsubscribeURL,
	})
	appName := strings.TrimSpace(s.branding.AppName)
	if appName == "" {
		appName = "Wisdom House"
	}
	subject := fmt.Sprintf("Welcome to %s", appName)
	_ = s.sender.SendHTML(sub.Email, subject, body)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeNotificationType(t string) string {
	norm := strings.ToLower(strings.TrimSpace(t))
	switch norm {
	case string(models.NotificationTypeUpdate), string(models.NotificationTypeEvent):
		return norm
	default:
		return ""
	}
}

func containsSensitiveNotificationContent(parts ...string) bool {
	joined := strings.ToLower(strings.Join(parts, " "))
	if strings.TrimSpace(joined) == "" {
		return false
	}

	denyList := []string{
		"testimonial removed",
		"testimonial approved",
		"approval request",
		"user deleted",
		"user suspended",
		"admin",
		"security alert",
		"password reset",
		"internal",
	}
	for _, term := range denyList {
		if strings.Contains(joined, term) {
			return true
		}
	}
	return false
}
