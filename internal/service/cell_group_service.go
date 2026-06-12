package service

import (
	"context"
	"fmt"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type CellGroupService interface {
	Create(ctx context.Context, g *models.CellGroup) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	Get(ctx context.Context, id string) (*models.CellGroup, error)
	List(ctx context.Context, campusID *string, activeOnly bool, limit, offset int) ([]models.CellGroup, int64, error)
	Delete(ctx context.Context, id string) error

	AddMember(ctx context.Context, groupID, memberID, role string) error
	RemoveMember(ctx context.Context, groupID, memberID string) error
	ListMembers(ctx context.Context, groupID string) ([]models.CellGroupMember, error)
	MemberGroups(ctx context.Context, memberID string) ([]models.CellGroup, error)

	CreateMeeting(ctx context.Context, m *models.CellGroupMeeting) error
	ListMeetings(ctx context.Context, groupID string, limit, offset int) ([]models.CellGroupMeeting, int64, error)
}

type cellGroupService struct {
	repo repository.CellGroupRepository
}

func NewCellGroupService(repo repository.CellGroupRepository) CellGroupService {
	return &cellGroupService{repo: repo}
}

func (s *cellGroupService) Create(ctx context.Context, g *models.CellGroup) error {
	if g.Name == "" {
		return fmt.Errorf("cell group name is required")
	}
	return s.repo.Create(ctx, g)
}

func (s *cellGroupService) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return s.repo.Update(ctx, id, updates)
}

func (s *cellGroupService) Get(ctx context.Context, id string) (*models.CellGroup, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *cellGroupService) List(ctx context.Context, campusID *string, activeOnly bool, limit, offset int) ([]models.CellGroup, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.List(ctx, campusID, activeOnly, limit, offset)
}

func (s *cellGroupService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *cellGroupService) AddMember(ctx context.Context, groupID, memberID, role string) error {
	if role == "" {
		role = "member"
	}
	m := &models.CellGroupMember{
		GroupID:  groupID,
		MemberID: memberID,
		Role:     role,
	}
	return s.repo.AddMember(ctx, m)
}

func (s *cellGroupService) RemoveMember(ctx context.Context, groupID, memberID string) error {
	return s.repo.RemoveMember(ctx, groupID, memberID)
}

func (s *cellGroupService) ListMembers(ctx context.Context, groupID string) ([]models.CellGroupMember, error) {
	return s.repo.ListMembers(ctx, groupID)
}

func (s *cellGroupService) MemberGroups(ctx context.Context, memberID string) ([]models.CellGroup, error) {
	return s.repo.MemberGroups(ctx, memberID)
}

func (s *cellGroupService) CreateMeeting(ctx context.Context, m *models.CellGroupMeeting) error {
	if m.GroupID == "" {
		return fmt.Errorf("group_id is required")
	}
	return s.repo.CreateMeeting(ctx, m)
}

func (s *cellGroupService) ListMeetings(ctx context.Context, groupID string, limit, offset int) ([]models.CellGroupMeeting, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListMeetings(ctx, groupID, limit, offset)
}
