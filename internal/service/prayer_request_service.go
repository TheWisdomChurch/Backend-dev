package service

import (
	"context"
	"fmt"
	"strings"

	"wisdomHouse-backend/internal/authutil"
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
	List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequest, int64, error)
	UpdateStatus(ctx context.Context, id, status string) error
	AssignTo(ctx context.Context, id, userID string) error
	AddNotes(ctx context.Context, id, notes string) error
	Delete(ctx context.Context, id string) error
}

type prayerRequestService struct {
	repo      repository.PrayerRequestRepository
	protector *authutil.Protector
}

func NewPrayerRequestService(repo repository.PrayerRequestRepository, authSecret string) PrayerRequestService {
	p, _ := authutil.NewProtector(authSecret)
	return &prayerRequestService{repo: repo, protector: p}
}

func (s *prayerRequestService) Submit(ctx context.Context, req SubmitPrayerRequest) (*models.PrayerRequest, error) {
	body := strings.TrimSpace(req.Request)
	if body == "" {
		return nil, fmt.Errorf("prayer request body is required")
	}

	enc, err := s.protector.EncryptString(body)
	if err != nil {
		return nil, fmt.Errorf("prayer request: encrypt request: %w", err)
	}

	pr := &models.PrayerRequest{
		MemberID:    req.MemberID,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Email:       req.Email,
		RequestEnc:  enc,
		Category:    req.Category,
		IsAnonymous: req.IsAnonymous,
		Status:      "pending",
	}
	if err := s.repo.Create(ctx, pr); err != nil {
		return nil, fmt.Errorf("prayer request: save: %w", err)
	}
	pr.Request = body
	return pr, nil
}

func (s *prayerRequestService) Get(ctx context.Context, id string) (*models.PrayerRequest, error) {
	pr, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.decrypt(pr)
	return pr, nil
}

func (s *prayerRequestService) List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequest, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, total, err := s.repo.List(ctx, status, category, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for i := range rows {
		s.decrypt(&rows[i])
	}
	return rows, total, nil
}

func (s *prayerRequestService) UpdateStatus(ctx context.Context, id, status string) error {
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
	enc, err := s.protector.EncryptString(notes)
	if err != nil {
		return fmt.Errorf("prayer request: encrypt notes: %w", err)
	}
	return s.repo.AddNotes(ctx, id, enc)
}

func (s *prayerRequestService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *prayerRequestService) decrypt(pr *models.PrayerRequest) {
	if pr == nil || s.protector == nil {
		return
	}
	if pr.RequestEnc != "" {
		if dec, err := s.protector.DecryptString(pr.RequestEnc); err == nil {
			pr.Request = dec
		}
	}
	if pr.NotesEnc != nil {
		if dec, err := s.protector.DecryptString(*pr.NotesEnc); err == nil {
			pr.Notes = &dec
		}
	}
}
