package service

import (
	"context"
	"fmt"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type MinistryService interface {
	Create(ctx context.Context, m *models.Ministry) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	Get(ctx context.Context, id string) (*models.Ministry, error)
	List(ctx context.Context, campusID, category *string, activeOnly bool, limit, offset int) ([]models.Ministry, int64, error)
	Delete(ctx context.Context, id string) error
	AddMember(ctx context.Context, ministryID, memberID, role string) error
	RemoveMember(ctx context.Context, ministryID, memberID string) error
	ListMembers(ctx context.Context, ministryID string) ([]models.MinistryMember, error)
	MemberMinistries(ctx context.Context, memberID string) ([]models.Ministry, error)
}

type ministryService struct {
	repo repository.MinistryRepository
}

func NewMinistryService(repo repository.MinistryRepository) MinistryService {
	return &ministryService{repo: repo}
}

func (s *ministryService) Create(ctx context.Context, m *models.Ministry) error {
	if m.Name == "" {
		return fmt.Errorf("ministry name is required")
	}
	return s.repo.Create(ctx, m)
}

func (s *ministryService) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.repo.Update(ctx, id, updates)
}

func (s *ministryService) Get(ctx context.Context, id string) (*models.Ministry, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *ministryService) List(ctx context.Context, campusID, category *string, activeOnly bool, limit, offset int) ([]models.Ministry, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.List(ctx, campusID, category, activeOnly, limit, offset)
}

func (s *ministryService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *ministryService) AddMember(ctx context.Context, ministryID, memberID, role string) error {
	if role == "" {
		role = "member"
	}
	return s.repo.AddMember(ctx, &models.MinistryMember{
		MinistryID: ministryID,
		MemberID:   memberID,
		Role:       role,
	})
}

func (s *ministryService) RemoveMember(ctx context.Context, ministryID, memberID string) error {
	return s.repo.RemoveMember(ctx, ministryID, memberID)
}

func (s *ministryService) ListMembers(ctx context.Context, ministryID string) ([]models.MinistryMember, error) {
	return s.repo.ListMembers(ctx, ministryID)
}

func (s *ministryService) MemberMinistries(ctx context.Context, memberID string) ([]models.Ministry, error) {
	return s.repo.MemberMinistries(ctx, memberID)
}
