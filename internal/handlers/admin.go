// internal/handlers/admin.go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AdminHandler struct {
	service service.AdminService
}

func NewAdminHandler(service service.AdminService) *AdminHandler {
	return &AdminHandler{service: service}
}

// GetDashboardStats returns admin dashboard statistics
func (h *AdminHandler) GetDashboardStats(c *gin.Context) {
	stats, err := h.service.GetDashboardStats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get dashboard stats")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Dashboard stats retrieved", stats)
}

// GetPendingTestimonials returns testimonials pending approval
func (h *AdminHandler) GetPendingTestimonials(c *gin.Context) {
	testimonials, err := h.service.GetPendingTestimonials()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get pending testimonials")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Pending testimonials retrieved", testimonials)
}

// GetAllUsers returns all users
func (h *AdminHandler) GetAllUsers(c *gin.Context) {
	users, err := h.service.GetAllUsers()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get users")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Users retrieved successfully", users)
}

// GetUserByID returns user by ID
func (h *AdminHandler) GetUserByID(c *gin.Context) {
	userID := c.Param("id")
	user, err := h.service.GetUserByID(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "User retrieved successfully", user)
}

// CreateUser creates a new user (admin only)
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req struct {
		FirstName string `json:"first_name" binding:"required"`
		LastName  string `json:"last_name" binding:"required"`
		Email     string `json:"email" binding:"required,email"`
		Password  string `json:"password" binding:"required,min=6"`
		Role      string `json:"role" binding:"required,oneof=user admin"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.service.CreateUser(req.FirstName, req.LastName, req.Email, req.Password, req.Role)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "User created successfully", user)
}

// UpdateUser updates user information
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	// Implementation
	utils.SuccessResponse(c, http.StatusOK, "Update user endpoint", nil)
}

// DeleteUser deletes a user
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	// Implementation
	utils.SuccessResponse(c, http.StatusOK, "Delete user endpoint", nil)
}