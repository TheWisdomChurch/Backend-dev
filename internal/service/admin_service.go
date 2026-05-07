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

type adminServiceImpl struct {
	adminRepo       repository.AdminRepository
	testimonialRepo repository.TestimonialRepository
	userRepo        repository.UserRepository
	approvalSvc     ApprovalService
	notifySvc       AdminNotificationService
	sender          EmailSender
	branding        email.Branding
}

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

func (s *adminServiceImpl) DeleteUser(id string) error {
	if s.userRepo == nil {
		return errors.New("user repository not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("user id is required")
	}
	if _, err := s.userRepo.FindByID(id); err != nil {
		return err
	}
	return s.userRepo.DeleteHard(id)
}

func (s *adminServiceImpl) UpdateUser(id string, data map[string]interface{}) (interface{}, error) {
	if s.userRepo == nil {
		return nil, errors.New("user repository not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
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
		if role == "super_admin" {
			return nil, errors.New("super admin role changes must be performed through a dedicated privileged workflow")
		}
		user.Role = role
		if role == "admin" && !user.AdminApproved {
			user.IsActive = false
			requestAdminApproval(s.approvalSvc, s.notifySvc, user)
		}
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
		// Pending admin accounts must not be activated through the generic update endpoint.
		if isAdminRole(user.Role) && !user.AdminApproved && v {
			return nil, errors.New("pending admin accounts must be approved by a super admin before activation")
		}
		user.IsActive = v
	}
	if v, ok := data["admin_approved"].(bool); ok {
		// Approval is intentionally not allowed through generic profile update.
		// Use ApproveUser so the approval ticket is completed and audit notifications remain accurate.
		if v && !user.AdminApproved {
			return nil, errors.New("use the super-admin approval endpoint to approve admin accounts")
		}
		if !v {
			user.AdminApproved = false
			if isAdminRole(user.Role) {
				user.IsActive = false
			}
		}
	}

	user.UpdatedAt = time.Now().UTC()
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	user.Password = ""
	return user, nil
}

func (s *adminServiceImpl) GetDashboardStats() (interface{}, error) {
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
		superAdminUsers       int
		pendingAdminApprovals int
		totpEnabledUsers      int
		approvedAdminUsers    int
	)

	for _, user := range users {
		totalUsers++
		role := normalizeAdminRoleForApproval(user.Role)
		if user.IsActive {
			activeUsers++
		}
		if role == "admin" || role == "super_admin" {
			adminUsers++
			if role == "super_admin" {
				superAdminUsers++
			}
			if user.AdminApproved {
				approvedAdminUsers++
			} else {
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
			500,
		)
		if listErr == nil {
			pendingRequests = len(items)
		}
	}

	securityScore := 0
	if adminUsers > 0 {
		approvalScore := float64(approvedAdminUsers) / float64(adminUsers) * 50
		mfaScore := float64(totpEnabledUsers) / float64(totalUsers) * 50
		securityScore = int(approvalScore + mfaScore)
	}

	return map[string]interface{}{
		"generatedAt":             time.Now().UTC(),
		"totalUsers":              totalUsers,
		"activeUsers":             activeUsers,
		"adminUsers":              adminUsers,
		"superAdminUsers":         superAdminUsers,
		"pendingAdminApprovals":   pendingAdminApprovals,
		"pendingApprovalRequests": pendingRequests,
		"totpEnabledUsers":        totpEnabledUsers,
		"approvedAdminUsers":      approvedAdminUsers,
		"securityScore":           securityScore,
	}, nil
}

func (s *adminServiceImpl) GetPendingTestimonials() (interface{}, error) {
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
	if s.userRepo == nil {
		return nil, errors.New("user repository not configured")
	}

	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}
	if role == "super_admin" {
		return nil, errors.New("super admin accounts cannot be created from the generic user screen")
	}
	if role != "admin" {
		return nil, errors.New("only admin accounts can be created from this workflow")
	}

	emailNorm := normalizeEmail(email)
	if emailNorm == "" {
		return nil, errors.New("invalid email")
	}

	existing, _ := s.userRepo.FindByEmail(emailNorm)
	if existing != nil {
		return nil, errors.New("user already exists")
	}

	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		FirstName:          strings.TrimSpace(firstName),
		LastName:           strings.TrimSpace(lastName),
		Email:              emailNorm,
		Password:           string(hashedPassword),
		Role:               "admin",
		IsActive:           false,
		AdminApproved:      false,
		EmailVerified:      false,
		PreferredMFAMethod: "email_otp",
		TOTPEnabled:        false,
	}

	if user.FirstName == "" || user.LastName == "" {
		return nil, errors.New("first name and last name are required")
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	requestAdminApproval(s.approvalSvc, s.notifySvc, user)

	user.Password = ""
	return user, nil
}

func (s *adminServiceImpl) ApproveUser(id string) (interface{}, error) {
	if s.userRepo == nil {
		return nil, errors.New("user repository not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("user id or approval request id is required")
	}

	var approvalReq *models.ApprovalRequest
	user, err := s.userRepo.FindByID(id)
	if err != nil || user == nil {
		if s.approvalSvc == nil {
			return nil, err
		}
		req, reqErr := s.approvalSvc.GetRequest(id)
		if reqErr != nil || req == nil || req.Type != models.ApprovalTypeAdminUser || req.EntityID == nil {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("admin account not found")
		}
		approvalReq = req
		user, err = s.userRepo.FindByID(*req.EntityID)
		if err != nil || user == nil {
			_, _ = s.approvalSvc.CompleteRequestByID(req.ID, models.ApprovalStatusDeleted, nil)
			return nil, errors.New("admin account no longer exists")
		}
	}

	role := normalizeAdminRoleForApproval(user.Role)
	if role != "admin" {
		return nil, errors.New("only admin accounts can be approved through this workflow")
	}

	if user.AdminApproved && user.IsActive {
		user.Password = ""
		return user, nil
	}

	user.AdminApproved = true
	user.IsActive = true
	user.EmailVerified = true
	if strings.TrimSpace(user.PreferredMFAMethod) == "" {
		user.PreferredMFAMethod = "email_otp"
	}
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	if s.approvalSvc != nil {
		if approvalReq != nil {
			_, _ = s.approvalSvc.CompleteRequestByID(approvalReq.ID, models.ApprovalStatusApproved, nil)
		} else {
			_, _ = s.approvalSvc.CompleteRequest(models.ApprovalTypeAdminUser, user.ID, models.ApprovalStatusApproved, nil)
		}
	}

	sendAdminApprovedEmail(s.sender, s.branding, user)
	user.Password = ""
	return user, nil
}
