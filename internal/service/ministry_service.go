package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type MinistryService interface {
	Create(ctx context.Context, m *models.Ministry) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	Get(ctx context.Context, id string) (*models.Ministry, error)
	List(ctx context.Context, campusID, category *string, activeOnly bool, limit, offset int) ([]models.Ministry, int64, error)
	Delete(ctx context.Context, id string) error
	AddMember(ctx context.Context, ministryID, memberID, role string) (*models.MinistryMember, error)
	RemoveMember(ctx context.Context, ministryID, memberID string) error
	ListMembers(ctx context.Context, ministryID string) ([]models.MinistryMember, error)
	MemberMinistries(ctx context.Context, memberID string) ([]models.Ministry, error)
	Structure(ctx context.Context, ministryID string) (*models.MinistryStructure, error)
	AssignWorkforceMember(ctx context.Context, ministryID string, req models.AssignMinistryWorkforceRequest) error
	UpdateWorkforceAssignment(ctx context.Context, ministryID, workforceMemberID string, req models.AssignMinistryWorkforceRequest) error
	RemoveWorkforceMember(ctx context.Context, ministryID, workforceMemberID string) error
	WorkforceMemberMinistries(ctx context.Context, workforceMemberID string) ([]models.Ministry, error)
}

type ministryService struct {
	repo          repository.MinistryRepository
	workforceRepo repository.WorkforceRepository
}

func NewMinistryService(repo repository.MinistryRepository, workforceRepo repository.WorkforceRepository) MinistryService {
	return &ministryService{repo: repo, workforceRepo: workforceRepo}
}

func validMinistryRole(role models.MinistryWorkforceRole) bool {
	switch role {
	case models.MinistryRoleHead, models.MinistryRoleDeputyHead, models.MinistryRoleCoordinator, models.MinistryRoleMember:
		return true
	default:
		return false
	}
}

func (s *ministryService) Structure(ctx context.Context, ministryID string) (*models.MinistryStructure, error) {
	ministry, err := s.repo.FindByID(ctx, ministryID)
	if err != nil {
		return nil, err
	}
	assignments, err := s.repo.ListWorkforceMembers(ctx, ministryID)
	if err != nil {
		return nil, err
	}
	out := &models.MinistryStructure{Ministry: *ministry, Total: len(assignments)}
	for _, assignment := range assignments {
		switch assignment.Role {
		case models.MinistryRoleHead:
			out.Heads = append(out.Heads, assignment)
		case models.MinistryRoleDeputyHead:
			out.DeputyHeads = append(out.DeputyHeads, assignment)
		case models.MinistryRoleCoordinator:
			out.Coordinators = append(out.Coordinators, assignment)
		default:
			out.Members = append(out.Members, assignment)
		}
	}
	if out.Heads == nil {
		out.Heads = []models.MinistryWorkforceMember{}
	}
	if out.DeputyHeads == nil {
		out.DeputyHeads = []models.MinistryWorkforceMember{}
	}
	if out.Coordinators == nil {
		out.Coordinators = []models.MinistryWorkforceMember{}
	}
	if out.Members == nil {
		out.Members = []models.MinistryWorkforceMember{}
	}
	return out, nil
}

func (s *ministryService) AssignWorkforceMember(ctx context.Context, ministryID string, req models.AssignMinistryWorkforceRequest) error {
	if _, err := s.repo.FindByID(ctx, ministryID); err != nil {
		return err
	}
	if s.workforceRepo == nil {
		return fmt.Errorf("workforce repository not configured")
	}
	if _, err := s.workforceRepo.GetByID(strings.TrimSpace(req.WorkforceMemberID)); err != nil {
		return fmt.Errorf("workforce member not found")
	}
	if req.Role == "" {
		req.Role = models.MinistryRoleMember
	}
	if !validMinistryRole(req.Role) {
		return fmt.Errorf("invalid ministry role")
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		req.Title = &title
	}
	return s.repo.AssignWorkforceMember(ctx, &models.MinistryWorkforceMember{MinistryID: ministryID, WorkforceMemberID: strings.TrimSpace(req.WorkforceMemberID), Role: req.Role, Title: req.Title})
}

func (s *ministryService) UpdateWorkforceAssignment(ctx context.Context, ministryID, workforceMemberID string, req models.AssignMinistryWorkforceRequest) error {
	if req.Role == "" || !validMinistryRole(req.Role) {
		return fmt.Errorf("valid ministry role is required")
	}
	updates := map[string]interface{}{"role": req.Role, "updated_at": time.Now().UTC()}
	if req.Title != nil {
		updates["title"] = strings.TrimSpace(*req.Title)
	}
	return s.repo.UpdateWorkforceAssignment(ctx, ministryID, workforceMemberID, updates)
}

func (s *ministryService) RemoveWorkforceMember(ctx context.Context, ministryID, workforceMemberID string) error {
	return s.repo.RemoveWorkforceMember(ctx, ministryID, workforceMemberID)
}

func (s *ministryService) WorkforceMemberMinistries(ctx context.Context, workforceMemberID string) ([]models.Ministry, error) {
	return s.repo.WorkforceMemberMinistries(ctx, workforceMemberID)
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

func (s *ministryService) AddMember(ctx context.Context, ministryID, memberID, role string) (*models.MinistryMember, error) {
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
