package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type adminServiceImpl struct {
	testimonialRepo repository.TestimonialRepository
	userRepo        repository.UserRepository
	auditLogRepo    repository.AuditLogRepository
	approvalSvc     ApprovalService
	notifySvc       AdminNotificationService
	sender          EmailSender
	branding        email.Branding
}

func NewAdminService(
	testimonialRepo repository.TestimonialRepository,
	userRepo repository.UserRepository,
	auditLogRepo repository.AuditLogRepository,
	approvalSvc ApprovalService,
	notifySvc AdminNotificationService,
	sender EmailSender,
	branding email.Branding,
) AdminService {
	return &adminServiceImpl{
		testimonialRepo: testimonialRepo,
		userRepo:        userRepo,
		auditLogRepo:    auditLogRepo,
		approvalSvc:     approvalSvc,
		notifySvc:       notifySvc,
		sender:          sender,
		branding:        branding,
	}
}

func normalizedAdminRole(role string) string {
	cleaned := strings.ToLower(strings.TrimSpace(role))
	cleaned = strings.ReplaceAll(cleaned, "-", "_")
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	return cleaned
}

func sanitizeAdminUser(user *models.User) *models.User {
	if user == nil {
		return nil
	}
	user.Password = ""
	return user
}

func (s *adminServiceImpl) ensureAdminApprovalRequest(user *models.User) error {
	if !needsAdminApproval(user) {
		return nil
	}
	if s.approvalSvc == nil {
		return errors.New("admin approval workflow is not configured")
	}

	existing, err := s.approvalSvc.ListRequests(
		[]models.ApprovalRequestType{models.ApprovalTypeAdminUser},
		[]models.ApprovalRequestStatus{models.ApprovalStatusPending},
		nil,
		nil,
		500,
	)
	if err == nil {
		for _, item := range existing {
			if item.EntityID != nil && strings.TrimSpace(*item.EntityID) == strings.TrimSpace(user.ID) {
				return nil
			}
		}
	}

	_, err = requestAdminApproval(s.approvalSvc, s.notifySvc, user)
	return err
}

func (s *adminServiceImpl) CreateUser(firstName, lastName, emailAddr, password, role string) (interface{}, error) {
	if s.userRepo == nil {
		return nil, errors.New("user repository not configured")
	}

	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}

	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	emailNorm := normalizeEmail(emailAddr)
	password = strings.TrimSpace(password)

	if firstName == "" || lastName == "" || emailNorm == "" || password == "" {
		return nil, errors.New("all required fields must be provided")
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}

	existing, _ := s.userRepo.FindByEmail(emailNorm)
	if existing != nil {
		return nil, errors.New("user already exists")
	}

	pendingAdmin := role == "admin"
	if pendingAdmin && s.approvalSvc == nil {
		return nil, errors.New("admin approval workflow is not configured")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &models.User{
		FirstName:          firstName,
		LastName:           lastName,
		Email:              emailNorm,
		Password:           string(hashedPassword),
		Role:               role,
		IsActive:           !pendingAdmin,
		AdminApproved:      !pendingAdmin,
		PreferredMFAMethod: "email_otp",
		TOTPEnabled:        false,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	if pendingAdmin {
		if err := s.ensureAdminApprovalRequest(user); err != nil {
			if delErr := s.userRepo.DeleteHard(user.ID); delErr != nil {
				slog.Error("admin_service: failed to rollback user creation after approval request failure",
					"user_id", user.ID, "error", delErr)
			}
			return nil, errors.New("failed to create admin approval request")
		}
	} else {
		sendAdminApprovedEmail(s.sender, s.branding, user)
	}

	return sanitizeAdminUser(user), nil
}

func (s *adminServiceImpl) findAdminUserOrRequest(id string) (*models.User, *models.ApprovalRequest, error) {
	if s.userRepo == nil {
		return nil, nil, errors.New("user repository not configured")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, errors.New("user id or approval request id is required")
	}

	user, err := s.userRepo.FindByID(id)
	if err == nil && user != nil {
		return user, nil, nil
	}

	if s.approvalSvc == nil {
		return nil, nil, errors.New("admin approval workflow is not configured")
	}

	req, reqErr := s.approvalSvc.GetRequest(id)
	if reqErr != nil || req == nil || req.Type != models.ApprovalTypeAdminUser || req.EntityID == nil {
		return nil, nil, errors.New("admin approval request not found")
	}

	user, err = s.userRepo.FindByID(strings.TrimSpace(*req.EntityID))
	if err != nil || user == nil {
		if _, completeErr := s.approvalSvc.CompleteRequestByID(req.ID, models.ApprovalStatusDeleted, nil); completeErr != nil {
			slog.Warn("admin_service: failed to mark orphaned approval request as deleted", "request_id", req.ID, "error", completeErr)
		}
		return nil, req, errors.New("admin account no longer exists")
	}

	return user, req, nil
}

func (s *adminServiceImpl) ApproveUser(id string) (interface{}, error) {
	user, req, err := s.findAdminUserOrRequest(id)
	if err != nil {
		return nil, err
	}

	if normalizedAdminRole(user.Role) != "admin" {
		return nil, errors.New("only admin accounts require approval")
	}

	alreadyApproved := user.AdminApproved && user.IsActive

	user.AdminApproved = true
	user.IsActive = true
	if strings.TrimSpace(user.PreferredMFAMethod) == "" {
		user.PreferredMFAMethod = "email_otp"
	}
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	if s.approvalSvc != nil {
		var completeErr error
		if req != nil {
			_, completeErr = s.approvalSvc.CompleteRequestByID(req.ID, models.ApprovalStatusApproved, nil)
		} else {
			_, completeErr = s.approvalSvc.CompleteRequest(models.ApprovalTypeAdminUser, user.ID, models.ApprovalStatusApproved, nil)
		}
		if completeErr != nil {
			slog.Warn("admin_service: failed to complete approval request on user approve", "user_id", user.ID, "error", completeErr)
		}
	}

	if !alreadyApproved {
		sendAdminApprovedEmail(s.sender, s.branding, user)
	}

	return sanitizeAdminUser(user), nil
}

func (s *adminServiceImpl) RejectUser(id string, reason string) (interface{}, error) {
	user, req, err := s.findAdminUserOrRequest(id)
	if err != nil {
		return nil, err
	}

	if normalizedAdminRole(user.Role) != "admin" {
		return nil, errors.New("only admin accounts can be rejected through this workflow")
	}
	if user.AdminApproved && user.IsActive {
		return nil, errors.New("approved admin accounts cannot be rejected; deactivate or delete the user instead")
	}

	user.AdminApproved = false
	user.IsActive = false
	if strings.TrimSpace(user.PreferredMFAMethod) == "" {
		user.PreferredMFAMethod = "email_otp"
	}
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	if s.approvalSvc != nil {
		var completeErr error
		if req != nil {
			_, completeErr = s.approvalSvc.CompleteRequestByID(req.ID, models.ApprovalStatusRejected, nil)
		} else {
			_, completeErr = s.approvalSvc.CompleteRequest(models.ApprovalTypeAdminUser, user.ID, models.ApprovalStatusRejected, nil)
		}
		if completeErr != nil {
			slog.Warn("admin_service: failed to complete approval request on user reject", "user_id", user.ID, "error", completeErr)
		}
	}

	sendAdminRejectedEmail(s.sender, s.branding, user, reason)
	return sanitizeAdminUser(user), nil
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
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	oldRole := normalizedAdminRole(user.Role)

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
		password := strings.TrimSpace(v)
		if password == "" {
			return nil, errors.New("password cannot be empty")
		}
		if len(password) < 8 {
			return nil, errors.New("password must be at least 8 characters")
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, errors.New("failed to hash password")
		}
		user.Password = string(hashedPassword)
	}

	nextRole := normalizedAdminRole(user.Role)

	if v, ok := data["admin_approved"].(bool); ok {
		user.AdminApproved = v
	}
	if v, ok := data["is_active"].(bool); ok {
		user.IsActive = v
	}

	approvalRequestNeeded := false
	if nextRole == "admin" && oldRole != "admin" {
		if s.approvalSvc == nil {
			return nil, errors.New("admin approval workflow is not configured")
		}
		user.AdminApproved = false
		user.IsActive = false
		approvalRequestNeeded = true
	}
	if nextRole == "admin" && !user.AdminApproved {
		user.IsActive = false
		approvalRequestNeeded = true
	}
	if nextRole == "admin" && user.AdminApproved {
		user.IsActive = true
	}
	if nextRole == "super_admin" {
		user.AdminApproved = true
		user.IsActive = true
	}
	if strings.TrimSpace(user.PreferredMFAMethod) == "" {
		user.PreferredMFAMethod = "email_otp"
	}

	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	if approvalRequestNeeded {
		if err := s.ensureAdminApprovalRequest(user); err != nil {
			return nil, errors.New("failed to create admin approval request")
		}
	}

	return sanitizeAdminUser(user), nil
}

func (s *adminServiceImpl) DeleteUser(id string) error {
	if s.userRepo == nil {
		return errors.New("user repository not configured")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("user id is required")
	}

	user, err := s.userRepo.FindByID(id)
	if err != nil || user == nil {
		return errors.New("user not found")
	}

	if normalizedAdminRole(user.Role) == "super_admin" {
		return errors.New("super admin accounts cannot be deleted through this endpoint")
	}

	return s.userRepo.DeleteHard(id)
}

func (s *adminServiceImpl) GetDashboardStats() (interface{}, error) {
	var totalUsers, totalTestimonials, pendingTestimonials int64

	if s.userRepo != nil {
		count, err := s.userRepo.GetTotalCount()
		if err != nil {
			return nil, err
		}
		totalUsers = count
	}

	if s.testimonialRepo != nil {
		count, err := s.testimonialRepo.GetTotalCount()
		if err != nil {
			return nil, err
		}
		totalTestimonials = count

		pending, err := s.testimonialRepo.GetPendingCount()
		if err != nil {
			return nil, err
		}
		pendingTestimonials = pending
	}

	pendingApprovals := 0
	if s.approvalSvc != nil {
		items, listErr := s.approvalSvc.ListRequests(nil, []models.ApprovalRequestStatus{models.ApprovalStatusPending}, nil, nil, 500)
		if listErr == nil {
			pendingApprovals = len(items)
		}
	}

	recentActivity := []models.AuditLog{}
	if s.auditLogRepo != nil {
		if recent, err := s.auditLogRepo.Recent(context.Background(), 10); err == nil {
			recentActivity = recent
		}
	}

	return map[string]interface{}{
		"total_users":          totalUsers,
		"total_testimonials":   totalTestimonials,
		"pending_testimonials": pendingTestimonials,
		"pending_approvals":    pendingApprovals,
		"recent_activity":      recentActivity,
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

	var totalUsers, activeUsers, adminUsers, superAdminUsers, privilegedUsers, pendingAdminApprovals, totpEnabledUsers, privilegedTOTPUsers, inactivePrivilegedUsers int

	for _, user := range users {
		totalUsers++
		role := normalizedAdminRole(user.Role)
		isPrivileged := role == "admin" || role == "super_admin"
		if user.IsActive {
			activeUsers++
		}
		if role == "admin" {
			adminUsers++
			if !user.AdminApproved || !user.IsActive {
				pendingAdminApprovals++
			}
		}
		if role == "super_admin" {
			superAdminUsers++
		}
		if isPrivileged {
			privilegedUsers++
			if !user.IsActive {
				inactivePrivilegedUsers++
			}
			if user.TOTPEnabled {
				privilegedTOTPUsers++
			}
		}
		if user.TOTPEnabled {
			totpEnabledUsers++
		}
	}

	pendingRequests := 0
	if s.approvalSvc != nil {
		items, listErr := s.approvalSvc.ListRequests(nil, []models.ApprovalRequestStatus{models.ApprovalStatusPending}, nil, nil, 500)
		if listErr == nil {
			pendingRequests = len(items)
		}
	}

	securityScore := 100
	if privilegedUsers > 0 {
		totpScore := int(float64(privilegedTOTPUsers) / float64(privilegedUsers) * 70)
		approvalScore := 30
		if pendingAdminApprovals > 0 || pendingRequests > 0 {
			approvalScore = 10
		}
		securityScore = totpScore + approvalScore
	}
	if securityScore < 0 {
		securityScore = 0
	}
	if securityScore > 100 {
		securityScore = 100
	}

	return map[string]interface{}{
		"generatedAt":                time.Now().UTC(),
		"totalUsers":                 totalUsers,
		"activeUsers":                activeUsers,
		"adminUsers":                 adminUsers,
		"superAdminUsers":            superAdminUsers,
		"privilegedUsers":            privilegedUsers,
		"pendingAdminApprovals":      pendingAdminApprovals,
		"pendingApprovalRequests":    pendingRequests,
		"totpEnabledUsers":           totpEnabledUsers,
		"privilegedTotpEnabledUsers": privilegedTOTPUsers,
		"inactivePrivilegedUsers":    inactivePrivilegedUsers,
		"securityScore":              securityScore,
	}, nil
}

func (s *adminServiceImpl) GetPendingTestimonials() (interface{}, error) {
	if s.testimonialRepo == nil {
		return []interface{}{}, nil
	}
	return s.testimonialRepo.FindByApprovalStatus(false)
}

func (s *adminServiceImpl) GetAllUsers() (interface{}, error) {
	if s.userRepo == nil {
		return []interface{}{}, nil
	}
	users, err := s.userRepo.FindAll()
	if err != nil {
		return nil, err
	}
	for index := range users {
		users[index].Password = ""
	}
	return users, nil
}

func (s *adminServiceImpl) GetUserByID(userID string) (interface{}, error) {
	if s.userRepo == nil {
		return nil, errors.New("user repository not configured")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user id is required")
	}
	user, err := s.userRepo.FindByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}
	return sanitizeAdminUser(user), nil
}
