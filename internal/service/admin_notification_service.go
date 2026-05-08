package service

import (
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type AdminNotificationService interface {
	NotifyRoles(input AdminNotificationInput) error
	ListForUser(userID string, limit int) ([]models.AdminNotification, int64, error)
	MarkRead(userID, id string) error
	MarkAllRead(userID string) error
}

type AdminNotificationInput struct {
	Type       string
	Title      string
	Message    string
	TicketCode *string
	EntityType *string
	EntityID   *string
	Roles      []string
}

type adminNotificationService struct {
	repo     *repository.AdminNotificationRepository
	userRepo repository.UserRepository
	sender   EmailSender
	branding email.Branding
}

func NewAdminNotificationService(
	repo *repository.AdminNotificationRepository,
	userRepo repository.UserRepository,
	sender EmailSender,
	branding email.Branding,
) AdminNotificationService {
	return &adminNotificationService{
		repo:     repo,
		userRepo: userRepo,
		sender:   sender,
		branding: branding,
	}
}

func (s *adminNotificationService) NotifyRoles(input AdminNotificationInput) error {
	if len(input.Roles) == 0 {
		return nil
	}
	users, err := s.userRepo.FindByRoles(input.Roles)
	if err != nil {
		return err
	}

	items := make([]models.AdminNotification, 0, len(users))
	for _, user := range users {
		if !user.IsActive {
			continue
		}
		items = append(items, models.AdminNotification{
			UserID:     user.ID,
			Type:       strings.TrimSpace(input.Type),
			Title:      strings.TrimSpace(input.Title),
			Message:    strings.TrimSpace(input.Message),
			TicketCode: input.TicketCode,
			EntityType: input.EntityType,
			EntityID:   input.EntityID,
		})

		if s.sender != nil && user.Email != "" {
			subject := input.Title
			recipientName := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
			message := input.Message
			if portal := strings.TrimSpace(s.branding.AdminPortalURL); portal != "" {
				message = fmt.Sprintf("%s\n\nOpen the admin portal: %s", message, portal)
			}
			body := email.RenderNotificationEmail(email.NotificationTemplateData{
				Branding:      s.branding,
				Title:         subject,
				Message:       message,
				RecipientName: &recipientName,
			})
			_ = s.sender.SendHTML(user.Email, subject, body)
		}
	}

	return s.repo.CreateMany(items)
}

func (s *adminNotificationService) ListForUser(userID string, limit int) ([]models.AdminNotification, int64, error) {
	items, err := s.repo.ListByUser(userID, limit)
	if err != nil {
		return nil, 0, err
	}
	unread, err := s.repo.CountUnread(userID)
	if err != nil {
		return items, 0, err
	}
	return items, unread, nil
}

func (s *adminNotificationService) MarkRead(userID, id string) error {
	return s.repo.MarkRead(userID, id, time.Now().UTC())
}

func (s *adminNotificationService) MarkAllRead(userID string) error {
	return s.repo.MarkAllRead(userID, time.Now().UTC())
}
