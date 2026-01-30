package handlers

import (
	"errors"
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
	service   service.AuthService
	jwtSecret []byte
	secure    bool
}

func NewAuthHandler(service service.AuthService) *AuthHandler {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Fail fast in production; in dev you still want visibility.
		// You can also panic here to crash on startup if preferred.
		fmt.Println("WARNING: JWT_SECRET not set")
	}

	secure := os.Getenv("ENVIRONMENT") == "production"

	return &AuthHandler{
		service:   service,
		jwtSecret: []byte(secret),
		secure:    secure,
	}
}

/* ============================================================================
   JWT
============================================================================ */

func (h *AuthHandler) generateToken(user *models.User) (string, error) {
	if len(h.jwtSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}

	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

/* ============================================================================
   Cookies
============================================================================ */

func (h *AuthHandler) setAuthCookie(c *gin.Context, token string, rememberMe bool) {
	// Session cookie by default
	maxAge := 0
	expires := time.Time{}

	if rememberMe {
		maxAge = int((7 * 24 * time.Hour) / time.Second)
		expires = time.Now().Add(7 * 24 * time.Hour)
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		Domain:   "", // keep empty unless you need cross-subdomain sharing
		MaxAge:   maxAge,
		Expires:  expires,
		Secure:   h.secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// Track inactivity (30 minutes)
	now := time.Now()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "last_activity",
		Value:    now.UTC().Format(time.RFC3339),
		Path:     "/",
		MaxAge:   int((30 * time.Minute) / time.Second),
		Expires:  now.Add(30 * time.Minute),
		Secure:   h.secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		Domain:   "",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   h.secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "last_activity",
		Value:    "",
		Path:     "/",
		Domain:   "",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   h.secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

/* ============================================================================
   Handlers
============================================================================ */

// Login establishes cookie-based session ONLY here
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required,email"`
		Password   string `json:"password" binding:"required,min=6"`
		RememberMe bool   `json:"rememberMe"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	meta := service.LoginMetadata{
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		DeviceID: func() string {
			if v, err := c.Cookie("device_id"); err == nil {
				return v
			}
			return ""
		}(),
	}

	result, err := h.service.Login(req.Email, req.Password, meta)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, service.ErrAdminPending) {
			status = http.StatusForbidden
		}
		utils.ErrorResponse(c, status, err.Error())
		return
	}

	if result.OTPRequired {
		utils.SuccessResponse(c, http.StatusAccepted, "Additional verification required", gin.H{
			"otp_required": true,
			"purpose":      result.OTPPurpose,
			"expires_at":   result.OTPExpiresAt,
			"action_url":   result.OTPActionURL,
			"email":        req.Email,
		})
		return
	}

	token, err := h.generateToken(result.User)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	h.setAuthCookie(c, token, req.RememberMe)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"user": gin.H{
				"id":         result.User.ID,
				"first_name": result.User.FirstName,
				"last_name":  result.User.LastName,
				"email":      result.User.Email,
				"role":       result.User.Role,
				"is_active":  result.User.IsActive,
				"created_at": result.User.CreatedAt,
				"updated_at": result.User.UpdatedAt,
			},
		},
	})
}

// Register creates account but does NOT authenticate or set cookies
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		FirstName string `json:"first_name" binding:"required,min=2,max=50"`
		LastName  string `json:"last_name" binding:"required,min=2,max=50"`
		Email     string `json:"email" binding:"required,email"`
		Password  string `json:"password" binding:"required,min=6"`
		Role      string `json:"role" binding:"required,oneof=admin super_admin"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	userData, err := h.service.Register(req.FirstName, req.LastName, req.Email, req.Password, req.Role)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	user, ok := userData.(*models.User)
	if !ok || user == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}

	// Safety: ensure registration never leaves the user authenticated
	h.clearAuthCookie(c)

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "Registration successful. Please log in.",
		"data": gin.H{
			"user": gin.H{
				"id":         user.ID,
				"first_name": user.FirstName,
				"last_name":  user.LastName,
				"email":      user.Email,
				"role":       user.Role,
				"is_active":  user.IsActive,
				"created_at": user.CreatedAt,
				"updated_at": user.UpdatedAt,
			},
		},
	})
}

func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	id, ok := userID.(string)
	if !ok || id == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid session")
		return
	}

	userData, err := h.service.GetUserByID(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	user, ok := userData.(*models.User)
	if !ok || user == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User retrieved successfully",
		"data": gin.H{
			"id":         user.ID,
			"first_name": user.FirstName,
			"last_name":  user.LastName,
			"email":      user.Email,
			"role":       user.Role,
			"is_active":  user.IsActive,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
	})
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	id, ok := userID.(string)
	if !ok || id == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid session")
		return
	}

	var req struct {
		FirstName string `json:"first_name" binding:"omitempty,min=2,max=50"`
		LastName  string `json:"last_name" binding:"omitempty,min=2,max=50"`
		Email     string `json:"email" binding:"omitempty,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	updatedUserData, err := h.service.UpdateProfile(id, req.FirstName, req.LastName, req.Email, "")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	user, ok := updatedUserData.(*models.User)
	if !ok || user == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Profile updated successfully",
		"data": gin.H{
			"id":         user.ID,
			"first_name": user.FirstName,
			"last_name":  user.LastName,
			"email":      user.Email,
			"role":       user.Role,
			"is_active":  user.IsActive,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
	})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	id, ok := userID.(string)
	if !ok || id == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid session")
		return
	}

	var req struct {
		CurrentPassword string `json:"currentPassword" binding:"required,min=6"`
		NewPassword     string `json:"newPassword" binding:"required,min=6"`
		ConfirmPassword string `json:"confirmPassword"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if req.ConfirmPassword != "" && req.NewPassword != req.ConfirmPassword {
		utils.ErrorResponse(c, http.StatusBadRequest, "New passwords do not match")
		return
	}

	if err := h.service.ChangePassword(id, req.CurrentPassword, req.NewPassword); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Password changed successfully",
	})
}

func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	id, ok := userID.(string)
	if !ok || id == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid session")
		return
	}

	if err := h.service.DeleteAccount(id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.clearAuthCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Account deleted successfully",
	})
}

func (h *AuthHandler) ClearData(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	id, ok := userID.(string)
	if !ok || id == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid session")
		return
	}

	if err := h.service.ClearData(id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User data cleared successfully",
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	id, ok := userID.(string)
	if !ok || id == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid session")
		return
	}

	userData, err := h.service.GetUserByID(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	user, ok := userData.(*models.User)
	if !ok || user == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}

	token, err := h.generateToken(user)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	// Refresh keeps session alive (treat as remembered)
	h.setAuthCookie(c, token, true)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Token refreshed successfully",
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	deviceID := ""
	if v, err := c.Cookie("device_id"); err == nil {
		deviceID = v
	}

	userID := ""
	if tokenCookie, err := c.Cookie("auth_token"); err == nil && tokenCookie != "" && len(h.jwtSecret) > 0 {
		if t, _, err := new(jwt.Parser).ParseUnverified(tokenCookie, jwt.MapClaims{}); err == nil {
			if claims, ok := t.Claims.(jwt.MapClaims); ok {
				if uid, ok := claims["user_id"].(string); ok {
					userID = uid
				}
			}
		} else {
			// try full parse to validate signature
			if t, err2 := jwt.Parse(tokenCookie, func(token *jwt.Token) (interface{}, error) {
				return h.jwtSecret, nil
			}); err2 == nil {
				if claims, ok := t.Claims.(jwt.MapClaims); ok && t.Valid {
					if uid, ok := claims["user_id"].(string); ok {
						userID = uid
					}
				}
			}
		}
	}

	if userID != "" && deviceID != "" {
		_ = h.service.UntrustDevice(userID, deviceID)
	}

	h.clearAuthCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Logout successful",
	})
}

func (h *AuthHandler) RequestPasswordReset(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	resp, err := h.service.RequestPasswordReset(req.Email, "")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password reset email sent", resp)
}

func (h *AuthHandler) ConfirmPasswordReset(c *gin.Context) {
	var req struct {
		Email           string `json:"email" binding:"required,email"`
		Code            string `json:"code" binding:"required,len=6"`
		Purpose         string `json:"purpose" binding:"required"`
		NewPassword     string `json:"newPassword" binding:"required,min=6"`
		ConfirmPassword string `json:"confirmPassword"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if req.ConfirmPassword != "" && req.NewPassword != req.ConfirmPassword {
		utils.ErrorResponse(c, http.StatusBadRequest, "New passwords do not match")
		return
	}

	if err := h.service.ResetPasswordWithOTP(req.Email, req.Code, req.Purpose, req.NewPassword); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password reset successful", nil)
}

func (h *AuthHandler) VerifyLoginOTP(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required,email"`
		Code       string `json:"code" binding:"required,len=6"`
		Purpose    string `json:"purpose" binding:"required"`
		RememberMe bool   `json:"rememberMe"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	meta := service.LoginMetadata{
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		DeviceID: func() string {
			if v, err := c.Cookie("device_id"); err == nil {
				return v
			}
			return ""
		}(),
	}

	user, err := h.service.VerifyLoginOTP(req.Email, req.Code, req.Purpose, meta)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	token, err := h.generateToken(user)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	h.setAuthCookie(c, token, req.RememberMe)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login verified",
		"data": gin.H{
			"user": gin.H{
				"id":         user.ID,
				"first_name": user.FirstName,
				"last_name":  user.LastName,
				"email":      user.Email,
				"role":       user.Role,
				"is_active":  user.IsActive,
				"created_at": user.CreatedAt,
				"updated_at": user.UpdatedAt,
			},
		},
	})
}
