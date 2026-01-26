// internal/service/admin_service.go
package service

import (
	"wisdomHouse-backend/internal/repository"
)

// AdminService implementation
type adminServiceImpl struct {
	adminRepo       repository.AdminRepository
	testimonialRepo repository.TestimonialRepository
	userRepo        repository.UserRepository
}

// DeleteUser implements [AdminService].
func (s *adminServiceImpl) DeleteUser(id string) error {
	panic("unimplemented")
}

// UpdateUser implements [AdminService].
func (s *adminServiceImpl) UpdateUser(id string, data map[string]interface{}) (interface{}, error) {
	panic("unimplemented")
}

// NewAdminService creates a new admin service
func NewAdminService(
	adminRepo repository.AdminRepository,
	testimonialRepo repository.TestimonialRepository,
	userRepo repository.UserRepository,
) AdminService {
	return &adminServiceImpl{
		adminRepo:       adminRepo,
		testimonialRepo: testimonialRepo,
		userRepo:        userRepo,
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

	// For now, return placeholder
	return map[string]interface{}{
		"id":         "user-id",
		"first_name": firstName,
		"last_name":  lastName,
		"email":      email,
		"role":       role,
		"created_at": "2024-01-14",
	}, nil
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
	user.AdminApproved = true
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	user.Password = ""
	return user, nil
}
