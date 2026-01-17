package handlers

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AuthHandler struct {
	service service.AuthService
}

func NewAuthHandler(service service.AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func generateToken(user *models.User) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}

	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// Login handles user login
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required,email"`
		Password   string `json:"password" binding:"required,min=6"`
		RememberMe bool   `json:"rememberMe"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Call service
	token, userData, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	user := userData.(*models.User)

	// Set HttpOnly cookie
	maxAge := 0 // Session cookie
	if req.RememberMe {
		maxAge = int(time.Hour * 24 * 30 / time.Second) // 30 days
	}
	secure := os.Getenv("ENVIRONMENT") == "production"

	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		MaxAge:   maxAge,
		Path:     "/",
		Domain:   "",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(c.Writer, cookie)

	// Return response without token
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"user": user,
		},
	})
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		FirstName  string `json:"first_name" binding:"required"`
		LastName   string `json:"last_name" binding:"required"`
		Email      string `json:"email" binding:"required,email"`
		Password   string `json:"password" binding:"required,min=6"`
		Role       string `json:"role" binding:"required,oneof=user admin"`
		RememberMe bool   `json:"rememberMe"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Call service
	userData, err := h.service.Register(req.FirstName, req.LastName, req.Email, req.Password, req.Role)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	user := userData.(*models.User)

	// Generate JWT for auto-login
	token, err := generateToken(user)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	// Set HttpOnly cookie
	maxAge := 0 // Session cookie
	if req.RememberMe {
		maxAge = int(time.Hour * 24 * 30 / time.Second) // 30 days
	}
	secure := os.Getenv("ENVIRONMENT") == "production"

	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		MaxAge:   maxAge,
		Path:     "/",
		Domain:   "",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(c.Writer, cookie)

	utils.SuccessResponse(c, http.StatusCreated, "Registration successful", user)
}

// GetCurrentUser returns current user info
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	userData, err := h.service.GetUserByID(userID.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "User retrieved successfully", userData)
}

// UpdateProfile handles user profile updates
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		FirstName string `json:"first_name" binding:"omitempty,min=2,max=50"`
		LastName  string `json:"last_name" binding:"omitempty,min=2,max=50"`
		Email     string `json:"email" binding:"omitempty,email"`
		Username  string `json:"username" binding:"omitempty,min=3,max=30"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Update user profile
	updatedUserData, err := h.service.UpdateProfile(userID.(string), req.FirstName, req.LastName, req.Email, req.Username)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Profile updated successfully", updatedUserData)
}

// ChangePassword handles password change
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"currentPassword" binding:"required,min=6"`
		NewPassword     string `json:"newPassword" binding:"required,min=6"`
		ConfirmPassword string `json:"confirmPassword" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Validate password confirmation
	if req.NewPassword != req.ConfirmPassword {
		utils.ErrorResponse(c, http.StatusBadRequest, "New passwords do not match")
		return
	}

	// Change password
	err := h.service.ChangePassword(userID.(string), req.CurrentPassword, req.NewPassword)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password changed successfully", nil)
}

// DeleteAccount handles account deletion
func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	// Delete account
	err := h.service.DeleteAccount(userID.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Clear cookie on delete
	c.SetCookie("auth_token", "", -1, "/", "", false, true)

	utils.SuccessResponse(c, http.StatusOK, "Account deleted successfully", nil)
}

// ClearData handles user data clearing
func (h *AuthHandler) ClearData(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	// Clear user data
	err := h.service.ClearData(userID.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "User data cleared successfully", nil)
}

// RefreshToken refreshes JWT token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// TODO: Implement token refresh logic
	utils.SuccessResponse(c, http.StatusOK, "Token refresh endpoint", gin.H{
		"message": "Token refresh logic to be implemented",
	})
}

// Logout handles user logout
func (h *AuthHandler) Logout(c *gin.Context) {
	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	utils.SuccessResponse(c, http.StatusOK, "Logout successful", nil)
}