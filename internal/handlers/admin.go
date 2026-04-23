package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type AdminHandler struct {
	svc service.AdminService
}

func NewAdminHandler(svc service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func (h *AdminHandler) GetDashboardStats(c *gin.Context) {
	stats, err := h.svc.GetDashboardStats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load dashboard stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Dashboard stats retrieved", stats)
}

func (h *AdminHandler) GetSecurityOverview(c *gin.Context) {
	stats, err := h.svc.GetSecurityOverview()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load security overview")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Security overview retrieved", stats)
}

func (h *AdminHandler) GetPendingTestimonials(c *gin.Context) {
	items, err := h.svc.GetPendingTestimonials()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load pending testimonials")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Pending testimonials retrieved", items)
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	users, err := h.svc.GetAllUsers()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load users")
		return
	}
	items, _ := users.([]models.User)
	resp := make([]adminUserResponse, 0, len(items))
	actorSuper := isSuperAdmin(c)
	for _, user := range items {
		resp = append(resp, presentAdminUser(user, actorSuper))
	}
	utils.SuccessResponse(c, http.StatusOK, "Users retrieved", resp)
}

func (h *AdminHandler) GetUserByID(c *gin.Context) {
	id, ok := userIDParam(c)
	if !ok {
		return
	}
	user, err := h.svc.GetUserByID(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}
	target, ok := user.(*models.User)
	if !ok || target == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}
	if !isSuperAdmin(c) && strings.EqualFold(strings.TrimSpace(target.Role), "super_admin") {
		utils.ErrorResponse(c, http.StatusForbidden, "Insufficient permissions")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User retrieved", presentAdminUser(*target, isSuperAdmin(c)))
}

type createUserRequest struct {
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=8"`
	Role      string `json:"role" binding:"required"`
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if !isSuperAdmin(c) && normalizeRole(req.Role) == "super_admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Only super admin can create super admin users")
		return
	}

	user, err := h.svc.CreateUser(req.FirstName, req.LastName, req.Email, req.Password, req.Role)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	created, ok := user.(*models.User)
	if !ok || created == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "User created", presentAdminUser(*created, isSuperAdmin(c)))
}

type updateUserRequest struct {
	FirstName      *string `json:"first_name"`
	LastName       *string `json:"last_name"`
	Email          *string `json:"email"`
	Password       *string `json:"password"`
	Role           *string `json:"role"`
	IsActive       *bool   `json:"is_active"`
	AdminApproved  *bool   `json:"admin_approved"`
	EmailConfirmed *bool   `json:"email_confirmed"`
}

func (h *AdminHandler) UpdateUser(c *gin.Context) {
	id, ok := userIDParam(c)
	if !ok {
		return
	}

	var req updateUserRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	currentRaw, currentExists := c.Get("user_id")
	currentUserID, _ := currentRaw.(string)

	targetRaw, targetErr := h.svc.GetUserByID(id)
	if targetErr != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}
	target, targetOK := targetRaw.(*models.User)
	if !targetOK || target == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}
	actorSuper := isSuperAdmin(c)
	if !actorSuper && normalizeRole(target.Role) == "super_admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Insufficient permissions")
		return
	}

	updates := make(map[string]interface{})
	addString := func(key string, val *string) {
		if val == nil {
			return
		}
		v := strings.TrimSpace(*val)
		if v == "" {
			return
		}
		updates[key] = v
	}

	addString("first_name", req.FirstName)
	addString("last_name", req.LastName)
	addString("email", req.Email)
	addString("password", req.Password)
	if req.Role != nil {
		nextRole := normalizeRole(*req.Role)
		if !actorSuper && nextRole == "super_admin" {
			utils.ErrorResponse(c, http.StatusForbidden, "Only super admin can assign super admin role")
			return
		}
		addString("role", req.Role)
	}
	if req.IsActive != nil {
		if currentExists && currentUserID == id && !*req.IsActive {
			utils.ErrorResponse(c, http.StatusBadRequest, "You cannot deactivate your own account")
			return
		}
		updates["is_active"] = *req.IsActive
	}
	if req.AdminApproved != nil {
		updates["admin_approved"] = *req.AdminApproved
	}
	if req.EmailConfirmed != nil {
		updates["email_confirmed"] = *req.EmailConfirmed
	}

	if len(updates) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "No fields to update")
		return
	}

	user, err := h.svc.UpdateUser(id, updates)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "User not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	updated, ok := user.(*models.User)
	if !ok || updated == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User updated", presentAdminUser(*updated, actorSuper))
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, ok := userIDParam(c)
	if !ok {
		return
	}
	currentRaw, _ := c.Get("user_id")
	currentUserID, _ := currentRaw.(string)
	if strings.TrimSpace(currentUserID) != "" && currentUserID == id {
		utils.ErrorResponse(c, http.StatusBadRequest, "You cannot delete your own account")
		return
	}
	targetRaw, targetErr := h.svc.GetUserByID(id)
	if targetErr != nil {
		if targetErr == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "User not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, targetErr.Error())
		return
	}
	target, targetOK := targetRaw.(*models.User)
	if !targetOK || target == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}
	if !isSuperAdmin(c) && normalizeRole(target.Role) == "super_admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Insufficient permissions")
		return
	}
	if err := h.svc.DeleteUser(id); err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "User not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User deleted", nil)
}

func (h *AdminHandler) ApproveUser(c *gin.Context) {
	id, ok := userIDParam(c)
	if !ok {
		return
	}
	user, err := h.svc.ApproveUser(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	approved, ok := user.(*models.User)
	if !ok || approved == nil {
		utils.SuccessResponse(c, http.StatusOK, "User approved", user)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User approved", presentAdminUser(*approved, isSuperAdmin(c)))
}

type adminUserResponse struct {
	ID                 string     `json:"id"`
	FirstName          string     `json:"first_name"`
	LastName           string     `json:"last_name"`
	Email              string     `json:"email"`
	Role               string     `json:"role"`
	IsActive           bool       `json:"is_active"`
	AdminApproved      bool       `json:"admin_approved"`
	PreferredMFAMethod string     `json:"preferred_mfa_method,omitempty"`
	TOTPEnabled        bool       `json:"totp_enabled"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func presentAdminUser(user models.User, includePrivileged bool) adminUserResponse {
	resp := adminUserResponse{
		ID:            user.ID,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		Email:         user.Email,
		Role:          user.Role,
		IsActive:      user.IsActive,
		AdminApproved: user.AdminApproved,
		LastLoginAt:   user.LastLoginAt,
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	}
	if includePrivileged {
		resp.PreferredMFAMethod = user.PreferredMFAMethod
		resp.TOTPEnabled = user.TOTPEnabled
	}
	return resp
}

func userIDParam(c *gin.Context) (string, bool) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "user id is required")
		return "", false
	}
	if _, err := uuid.Parse(id); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "user id must be a valid UUID")
		return "", false
	}
	return id, true
}

func isSuperAdmin(c *gin.Context) bool {
	roleRaw, exists := c.Get("role")
	if !exists {
		return false
	}
	role, ok := roleRaw.(string)
	if !ok {
		return false
	}
	return normalizeRole(role) == "super_admin"
}

func normalizeRole(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
