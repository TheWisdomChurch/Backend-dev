// internal/handlers/auth.go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AuthHandler struct {
	service service.AuthService
}

func NewAuthHandler(service service.AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

// Login handles user login
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Call service
	token, user, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Return response with token
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"token": token,
			"user":  user,
		},
	})
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
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

	// Call service
	user, err := h.service.Register(req.FirstName, req.LastName, req.Email, req.Password, req.Role)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Registration successful", user)
}

// GetCurrentUser returns current user info
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	user, err := h.service.GetUserByID(userID.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "User retrieved successfully", user)
}

// RefreshToken refreshes JWT token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// Implement token refresh logic
	utils.SuccessResponse(c, http.StatusOK, "Token refresh endpoint", gin.H{
		"message": "Token refresh logic to be implemented",
	})
}

// Logout handles user logout
func (h *AuthHandler) Logout(c *gin.Context) {
	// Implement logout logic (e.g., blacklist token)
	utils.SuccessResponse(c, http.StatusOK, "Logout successful", nil)
}