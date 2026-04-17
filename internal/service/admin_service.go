// internal/service/admin_service.go
package service

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

// AdminService implementation
type adminServiceImpl struct {
	adminRepo       repository.AdminRepository
	testimonialRepo repository.TestimonialRepository
	userRepo        repository.UserRepository
	approvalSvc     ApprovalService
	notifySvc       AdminNotificationService
	sender          EmailSender
	branding        email.Branding
}

// DeleteUser implements [AdminService].
func (s *adminServiceImpl) DeleteUser(id string) error {
	if s.userRepo == nil {
		return errors.New("user repository not configured")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("user id is required")
	}
	if _, err := s.userRepo.FindByID(id); err != nil {
		return err
	}
	return s.userRepo.DeleteHard(id)
}

// UpdateUser implements [AdminService].
func (s *adminServiceImpl) UpdateUser(id string, data map[string]interface{}) (interface{}, error) {
	if s.userRepo == nil {
		return nil, errors.New("user repository not configured")
	}
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("user id is required")
	}
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if v, ok := data["first_name"].(string); ok && strings.TrimSpace(v) != "" {
		user.FirstName = strings.TrimSpace(v)
	}
	if v, ok := data["last_name"].(string); ok && strings.TrimSpace(v) != "" {
		user.LastName = strings.TrimSpace(v)
	}
	if v, ok := data["email"].(string); ok && strings.TrimSpace(v) != "" {
		emailNorm := normalizeEmail(v)
		if emailNorm == "" {
			return nil, errors.New("invalid email")
		}
		existing, _ := s.userRepo.FindByEmail(emailNorm)
		if existing != nil && existing.ID != user.ID {
			return nil, errors.New("email already in use")
		}
		user.Email = emailNorm
	}
	if v, ok := data["role"].(string); ok && strings.TrimSpace(v) != "" {
		role, err := normalizeRole(v)
		if err != nil {
			return nil, err
		}
		user.Role = role
	}
	if v, ok := data["password"].(string); ok {
		if strings.TrimSpace(v) == "" {
			return nil, errors.New("password cannot be empty")
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(v), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.Password = string(hashedPassword)
	}
	if v, ok := data["is_active"].(bool); ok {
		user.IsActive = v
	}
	if v, ok := data["admin_approved"].(bool); ok {
		user.AdminApproved = v
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	user.Password = ""
	return user, nil
}

// NewAdminService creates a new admin service
func NewAdminService(
	adminRepo repository.AdminRepository,
	testimonialRepo repository.TestimonialRepository,
	userRepo repository.UserRepository,
	approvalSvc ApprovalService,
	notifySvc AdminNotificationService,
	sender EmailSender,
	branding email.Branding,
) AdminService {
	return &adminServiceImpl{
		adminRepo:       adminRepo,
		testimonialRepo: testimonialRepo,
		userRepo:        userRepo,
		approvalSvc:     approvalSvc,
		notifySvc:       notifySvc,
		sender:          sender,
		branding:        branding,
	}
}

func (s *adminServiceImpl) GetDashboardStats() (interface{}, error) {
	// For now, return placeholder data
	// You can implement real database queries later
	return map[string]interface{}{
		"total_testimonials": 0,
		"pending_approvals":  0,
		"total_users":        0,
		"recent_activity":    []map[string]interface{}{},
	}, nil
}

func (s *adminServiceImpl) GetSecurityOverview() (interface{}, error) {
	if s.userRepo == nil {
		return map[string]interface{}{}, nil
	}

	users, err := s.userRepo.FindAll()
	if err != nil {
		return nil, err
	}

	var (
		totalUsers            int
		activeUsers           int
		adminUsers            int
		pendingAdminApprovals int
		totpEnabledUsers      int
	)

	for _, user := range users {
		totalUsers++
		if user.IsActive {
			activeUsers++
		}
		if user.Role == "admin" {
			adminUsers++
			if !user.AdminApproved {
				pendingAdminApprovals++
			}
		}
		if user.TOTPEnabled {
			totpEnabledUsers++
		}
	}

	pendingRequests := 0
	if s.approvalSvc != nil {
		items, listErr := s.approvalSvc.ListRequests(
			nil,
			[]models.ApprovalRequestStatus{models.ApprovalStatusPending},
			nil,
			nil,
			200,
		)
		if listErr == nil {
			pendingRequests = len(items)
		}
	}

	securityScore := 0
	if totalUsers > 0 {
		securityScore = int(float64(totpEnabledUsers) / float64(totalUsers) * 100)
	}

	return map[string]interface{}{
		"generatedAt":            time.Now().UTC(),
		"totalUsers":             totalUsers,
		"activeUsers":            activeUsers,
		"adminUsers":             adminUsers,
		"pendingAdminApprovals":  pendingAdminApprovals,
		"pendingApprovalRequests": pendingRequests,
		"totpEnabledUsers":       totpEnabledUsers,
		"securityScore":          securityScore,
	}, nil
}

func (s *adminServiceImpl) GetPendingTestimonials() (interface{}, error) {
	// Return pending testimonials
	return s.testimonialRepo.FindByApprovalStatus(false)
}

func (s *adminServiceImpl) GetAllUsers() (interface{}, error) {
	if s.userRepo == nil {
		return []interface{}{}, nil
	}
	return s.userRepo.FindAll()
}

func (s *adminServiceImpl) GetUserByID(userID string) (interface{}, error) {
	if s.userRepo == nil {
		return map[string]interface{}{}, nil
	}
	return s.userRepo.FindByID(userID)
}

func (s *adminServiceImpl) CreateUser(firstName, lastName, email, password, role string) (interface{}, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}

	if s.userRepo == nil {
		return nil, errors.New("user repository not configured")
	}

	emailNorm := normalizeEmail(email)
	if emailNorm == "" {
		return nil, errors.New("invalid email")
	}

	existing, _ := s.userRepo.FindByEmail(emailNorm)
	if existing != nil {
		return nil, errors.New("user already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		FirstName: strings.TrimSpace(firstName),
		LastName:  strings.TrimSpace(lastName),
		Email:     emailNorm,
		Password:  string(hashedPassword),
		Role:      role,
		IsActive:  true,
		AdminApproved: func() bool {
			if role == "admin" {
				return false
			}
			return true
		}(),
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	if needsAdminApproval(user) {
		requestAdminApproval(s.approvalSvc, s.notifySvc, user)
	}

	user.Password = ""
	return user, nil
}

func (s *adminServiceImpl) ApproveUser(id string) (interface{}, error) {
	if s.userRepo == nil {
		return nil, nil
	}
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if user.Role != "admin" {
		return nil, nil
	}
	if user.AdminApproved {
		user.Password = ""
		return user, nil
	}
	user.AdminApproved = true
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	if s.approvalSvc != nil {
		_, _ = s.approvalSvc.CompleteRequest(models.ApprovalTypeAdminUser, user.ID, models.ApprovalStatusApproved, nil)
	}
	sendAdminApprovedEmail(s.sender, s.branding, user)
	user.Password = ""
	return user, nil
}
