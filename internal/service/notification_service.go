package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

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
	ListSubscribers(page, limit int) ([]models.Subscriber, int64, error)
	SendNotification(req *models.SendNotificationRequest) (*models.SendNotificationResult, error)
}

type notificationService struct {
	subscribers   *repository.SubscriberRepository
	notifications *repository.NotificationRepository
	eventRepo     *repository.EventRepository
	sender        EmailSender
	branding      email.Branding
}

func NewNotificationService(
	subscriberRepo *repository.SubscriberRepository,
	notificationRepo *repository.NotificationRepository,
	eventRepo *repository.EventRepository,
	sender EmailSender,
	branding email.Branding,
) NotificationService {
	return &notificationService{
		subscribers:   subscriberRepo,
		notifications: notificationRepo,
		eventRepo:     eventRepo,
		sender:        sender,
		branding:      branding,
	}
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
	return s.subscribers.Update(sub)
}

func (s *notificationService) ListSubscribers(page, limit int) ([]models.Subscriber, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.subscribers.List(offset, limit)
}

func (s *notificationService) SendNotification(req *models.SendNotificationRequest) (*models.SendNotificationResult, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}

	nType := normalizeNotificationType(req.Type)
	if nType == "" {
		return nil, errors.New("type must be 'update' or 'event'")
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
			Title:         notification.Title,
			Message:       notification.Message,
			Event:         event,
			RecipientName: sub.Name,
		})

		if err := s.sender.SendHTML(sub.Email, notification.Subject, body); err != nil {
			errMsg := err.Error()
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
		result.Sent++
	}

	return result, nil
}

func (s *notificationService) sendSubscriberWelcome(sub *models.Subscriber) {
	if s.sender == nil {
		return
	}
	unsubscribeURL := ""
	if strings.TrimSpace(s.branding.PublicURL) != "" {
		unsubscribeURL = strings.TrimRight(s.branding.PublicURL, "/") + "/api/v1/subscribers/unsubscribe?email=" + url.QueryEscape(sub.Email)
	}
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
