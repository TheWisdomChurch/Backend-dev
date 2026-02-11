package service

import (
	"errors"
	"fmt"
	"strings"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type LeadershipService interface {
	List(page, limit int, role, status string) ([]models.LeadershipMember, int64, error)
	ListApproved(role string) ([]models.LeadershipMember, error)
	Apply(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error)
	Create(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error)
	Update(id string, req *models.UpdateLeadershipRequest) (*models.LeadershipMember, error)
	Approve(id string) (*models.LeadershipMember, error)
	Delete(id string) error
}

type leadershipService struct {
	repo      repository.LeadershipRepository
	notifySvc AdminNotificationService
}

func NewLeadershipService(repo repository.LeadershipRepository, notifySvc AdminNotificationService) LeadershipService {
	return &leadershipService{repo: repo, notifySvc: notifySvc}
}

func (s *leadershipService) List(page, limit int, role, status string) ([]models.LeadershipMember, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.List(offset, limit, role, status)
}

func (s *leadershipService) ListApproved(role string) ([]models.LeadershipMember, error) {
	return s.repo.ListApproved(role)
}

func (s *leadershipService) Apply(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error) {
	member, err := s.createWithStatus(req, models.LeadershipStatusPending)
	if err != nil {
		return nil, err
	}
	s.notifyNewApplication(member)
	return member, nil
}

func (s *leadershipService) Create(req *models.CreateLeadershipRequest) (*models.LeadershipMember, error) {
	status := req.Status
	if status == "" {
		status = models.LeadershipStatusPending
	}
	return s.createWithStatus(req, status)
}

func (s *leadershipService) Update(id string, req *models.UpdateLeadershipRequest) (*models.LeadershipMember, error) {
	updates := map[string]interface{}{}

	if req.FirstName != nil {
		updates["first_name"] = strings.TrimSpace(*req.FirstName)
	}
	if req.LastName != nil {
		updates["last_name"] = strings.TrimSpace(*req.LastName)
	}
	if req.Email != nil {
		updates["email"] = strings.TrimSpace(*req.Email)
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.Role != nil {
		if !isValidLeadershipRole(*req.Role) {
			return nil, errors.New("invalid leadership role")
		}
		updates["role"] = *req.Role
	}
	if req.Status != nil {
		if !isValidLeadershipStatus(*req.Status) {
			return nil, errors.New("invalid leadership status")
		}
		updates["status"] = *req.Status
	}
	if req.Bio != nil {
		updates["bio"] = req.Bio
	}
	if req.ImageURL != nil {
		updates["image_url"] = req.ImageURL
	}

	if len(updates) == 0 {
		return nil, errors.New("no updates provided")
	}

	return s.repo.Update(id, updates)
}

func (s *leadershipService) Approve(id string) (*models.LeadershipMember, error) {
	member, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if member.Status == models.LeadershipStatusApproved {
		return member, nil
	}
	return s.repo.Update(id, map[string]interface{}{"status": models.LeadershipStatusApproved})
}

func (s *leadershipService) Delete(id string) error {
	return s.repo.Delete(id)
}

func (s *leadershipService) createWithStatus(
	req *models.CreateLeadershipRequest,
	status models.LeadershipStatus,
) (*models.LeadershipMember, error) {
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	if firstName == "" || lastName == "" {
		return nil, errors.New("firstName and lastName are required")
	}
	if !isValidLeadershipRole(req.Role) {
		return nil, errors.New("invalid leadership role")
	}
	if !isValidLeadershipStatus(status) {
		return nil, errors.New("invalid leadership status")
	}

	member := &models.LeadershipMember{
		FirstName: firstName,
		LastName:  lastName,
		Email:     strings.TrimSpace(req.Email),
		Phone:     strings.TrimSpace(req.Phone),
		Role:      req.Role,
		Status:    status,
		Bio:       req.Bio,
		ImageURL:  req.ImageURL,
	}

	if err := s.repo.Create(member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *leadershipService) notifyNewApplication(member *models.LeadershipMember) {
	if s.notifySvc == nil || member == nil {
		return
	}
	fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	title := "New leadership application"
	message := fmt.Sprintf("%s applied for leadership (%s).", fullName, member.Role)
	entityType := "leadership"
	entityID := member.ID
	_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "leadership_application",
		Title:      title,
		Message:    message,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"admin", "super_admin"},
	})
}

func isValidLeadershipRole(role models.LeadershipRole) bool {
	switch role {
	case models.LeadershipRoleAssociatePastor,
		models.LeadershipRoleDeacon,
		models.LeadershipRoleDeaconess,
		models.LeadershipRoleReverend:
		return true
	default:
		return false
	}
}

func isValidLeadershipStatus(status models.LeadershipStatus) bool {
	switch status {
	case models.LeadershipStatusPending,
		models.LeadershipStatusApproved:
		return true
	default:
		return false
	}
}
