package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

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
	utils.SuccessResponse(c, http.StatusOK, "Users retrieved", users)
}

func (h *AdminHandler) GetUserByID(c *gin.Context) {
	id := c.Param("id")
	user, err := h.svc.GetUserByID(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User retrieved", user)
}

type createUserRequest struct {
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6"`
	Role      string `json:"role" binding:"required"`
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	user, err := h.svc.CreateUser(req.FirstName, req.LastName, req.Email, req.Password, req.Role)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "User created", user)
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
	id := c.Param("id")

	var req updateUserRequest
	if !validation.BindJSON(c, &req) {
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
	addString("role", req.Role)
	if req.IsActive != nil {
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
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User updated", user)
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.DeleteUser(id); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User deleted", nil)
}

func (h *AdminHandler) ApproveUser(c *gin.Context) {
	id := c.Param("id")
	user, err := h.svc.ApproveUser(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "User approved", user)
}
