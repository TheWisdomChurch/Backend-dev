package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type SubmitPrayerRequest struct {
	MemberID    *string `json:"member_id,omitempty"`
	FirstName   string  `json:"first_name" binding:"required"`
	LastName    string  `json:"last_name" binding:"required"`
	Email       string  `json:"email,omitempty"`
	Request     string  `json:"request" binding:"required"`
	Category    string  `json:"category,omitempty"`
	IsAnonymous bool    `json:"is_anonymous,omitempty"`
}

type PrayerRequestService interface {
	Submit(ctx context.Context, req SubmitPrayerRequest) (*models.PrayerRequest, error)
	Get(ctx context.Context, id string) (*models.PrayerRequest, error)
	List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequestSummary, int64, error)
	UpdateStatus(ctx context.Context, id, status string) error
	AssignTo(ctx context.Context, id, userID string) error
	AddNotes(ctx context.Context, id, notes string) error
	Delete(ctx context.Context, id string) error
}

type prayerRequestService struct {
	repo              repository.PrayerRequestRepository
	protector         *authutil.Protector
	sender            EmailSender
	branding          email.Branding
	notificationEmail string
}

func NewPrayerRequestService(repo repository.PrayerRequestRepository, authSecret string, sender EmailSender, branding email.Branding, notificationEmail string) (PrayerRequestService, error) {
	if repo == nil {
		return nil, errors.New("prayer request repository is required")
	}
	p, err := authutil.NewProtector(authSecret)
	if err != nil {
		return nil, fmt.Errorf("configure prayer request encryption: %w", err)
	}
	recipient := strings.ToLower(strings.TrimSpace(notificationEmail))
	if recipient != "" {
		parsed, err := mail.ParseAddress(recipient)
		if err != nil || !strings.EqualFold(parsed.Address, recipient) {
			return nil, errors.New("PRAYER_NOTIFICATION_EMAIL must be a valid email address")
		}
	}
	return &prayerRequestService{repo: repo, protector: p, sender: sender, branding: branding, notificationEmail: recipient}, nil
}

func (s *prayerRequestService) Submit(ctx context.Context, req SubmitPrayerRequest) (*models.PrayerRequest, error) {
	body := strings.TrimSpace(req.Request)
	if body == "" {
		return nil, fmt.Errorf("prayer request body is required")
	}
	if len([]rune(body)) > 10_000 {
		return nil, errors.New("prayer request body is too long")
	}
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	if firstName == "" || lastName == "" || len([]rune(firstName)) > 100 || len([]rune(lastName)) > 100 {
		return nil, errors.New("valid first and last names are required")
	}
	emailAddr := strings.ToLower(strings.TrimSpace(req.Email))
	if emailAddr != "" {
		parsed, err := mail.ParseAddress(emailAddr)
		if err != nil || !strings.EqualFold(parsed.Address, emailAddr) {
			return nil, errors.New("email is invalid")
		}
	}
	category := strings.TrimSpace(req.Category)
	if len([]rune(category)) > 100 {
		return nil, errors.New("category is too long")
	}

	enc, err := s.protector.EncryptString(body)
	if err != nil {
		return nil, fmt.Errorf("prayer request: encrypt request: %w", err)
	}

	pr := &models.PrayerRequest{
		MemberID:    req.MemberID,
		FirstName:   firstName,
		LastName:    lastName,
		Email:       emailAddr,
		RequestEnc:  enc,
		Category:    category,
		IsAnonymous: req.IsAnonymous,
		Status:      "pending",
	}
	if err := s.repo.Create(ctx, pr); err != nil {
		return nil, fmt.Errorf("prayer request: save: %w", err)
	}
	s.notifyRecipient(pr)
	pr.Request = body
	return pr, nil
}

func (s *prayerRequestService) Get(ctx context.Context, id string) (*models.PrayerRequest, error) {
	pr, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.decrypt(pr); err != nil {
		return nil, err
	}
	return pr, nil
}

func (s *prayerRequestService) List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequestSummary, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, total, err := s.repo.List(ctx, status, category, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (s *prayerRequestService) UpdateStatus(ctx context.Context, id, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	allowed := map[string]bool{"pending": true, "praying": true, "answered": true, "closed": true}
	if !allowed[status] {
		return fmt.Errorf("invalid status %q", status)
	}
	return s.repo.UpdateStatus(ctx, id, status)
}

func (s *prayerRequestService) AssignTo(ctx context.Context, id, userID string) error {
	return s.repo.AssignTo(ctx, id, userID)
}

func (s *prayerRequestService) AddNotes(ctx context.Context, id, notes string) error {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return errors.New("notes are required")
	}
	if len([]rune(notes)) > 10_000 {
		return errors.New("notes are too long")
	}
	enc, err := s.protector.EncryptString(notes)
	if err != nil {
		return fmt.Errorf("prayer request: encrypt notes: %w", err)
	}
	return s.repo.AddNotes(ctx, id, enc)
}

func (s *prayerRequestService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *prayerRequestService) decrypt(pr *models.PrayerRequest) error {
	if pr == nil || s.protector == nil {
		return errors.New("prayer request decryption is not configured")
	}
	if pr.RequestEnc != "" {
		dec, err := s.protector.DecryptString(pr.RequestEnc)
		if err != nil {
			return fmt.Errorf("decrypt prayer request: %w", err)
		}
		pr.Request = dec
	}
	if pr.NotesEnc != nil {
		dec, err := s.protector.DecryptString(*pr.NotesEnc)
		if err != nil {
			return fmt.Errorf("decrypt prayer request notes: %w", err)
		}
		pr.Notes = &dec
	}
	return nil
}

func (s *prayerRequestService) notifyRecipient(pr *models.PrayerRequest) {
	if pr == nil || s.sender == nil || s.notificationEmail == "" {
		return
	}
	adminURL := strings.TrimRight(strings.TrimSpace(s.branding.AdminPortalURL), "/")
	if adminURL != "" {
		adminURL += "/prayer-requests/" + url.PathEscape(pr.ID)
	}
	body := email.RenderPrayerRequestNotificationEmail(email.PrayerRequestNotificationTemplateData{
		Branding: s.branding, ReferenceID: pr.ID, Category: pr.Category,
		SubmittedAt: pr.CreatedAt.UTC().Format(time.RFC1123), AdminViewURL: adminURL,
	})
	if err := s.sender.SendHTML(s.notificationEmail, "New confidential prayer request", body); err != nil {
		// Persistence is authoritative: never tell the public client submission
		// failed after the encrypted record was already committed.
		slog.Error("prayer request notification failed", "prayer_request_id", pr.ID, "error", err)
	}
}
