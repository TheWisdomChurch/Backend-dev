// internal/handlers/auth_handler.go
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type AuthHandlerOptions struct {
	JWTSecret                    string
	Secure                       bool
	AccessTokenTTL               time.Duration
	RememberMeTTL                time.Duration
	SessionIdleTimeout           time.Duration
	RememberedSessionIdleTimeout time.Duration
	PostLoginRedirectURL         string
	AuthSecretKey                string
	GoogleClientID               string
	GoogleClientSecret           string
	GoogleRedirectURL            string
	GoogleHostedDomain           string
}

type AuthHandler struct {
	service                      service.AuthService
	jwtSecret                    []byte
	secure                       bool
	accessTokenTTL               time.Duration
	rememberMeTTL                time.Duration
	sessionIdleTimeout           time.Duration
	rememberedSessionIdleTimeout time.Duration
	postLoginRedirectURL         string
	googleClientID               string
	googleClientSecret           string
	googleRedirectURL            string
	googleHostedDomain           string
	protector                    *authutil.Protector
	httpClient                   *http.Client
}

func NewAuthHandler(service service.AuthService, opts AuthHandlerOptions) *AuthHandler {
	if strings.TrimSpace(opts.JWTSecret) == "" {
		fmt.Println("WARNING: JWT_SECRET not set")
	}

	if opts.AccessTokenTTL <= 0 {
		opts.AccessTokenTTL = 24 * time.Hour
	}
	if opts.RememberMeTTL <= 0 {
		opts.RememberMeTTL = 30 * 24 * time.Hour
	}
	if opts.SessionIdleTimeout <= 0 {
		opts.SessionIdleTimeout = 30 * time.Minute
	}
	if opts.RememberedSessionIdleTimeout <= 0 {
		opts.RememberedSessionIdleTimeout = 7 * 24 * time.Hour
	}

	authSecret := strings.TrimSpace(opts.AuthSecretKey)
	if authSecret == "" {
		authSecret = strings.TrimSpace(opts.JWTSecret)
	}

	var protector *authutil.Protector
	if p, err := authutil.NewProtector(authSecret); err == nil {
		protector = p
	}

	return &AuthHandler{
		service:                      service,
		jwtSecret:                    []byte(opts.JWTSecret),
		secure:                       opts.Secure,
		accessTokenTTL:               opts.AccessTokenTTL,
		rememberMeTTL:                opts.RememberMeTTL,
		sessionIdleTimeout:           opts.SessionIdleTimeout,
		rememberedSessionIdleTimeout: opts.RememberedSessionIdleTimeout,
		postLoginRedirectURL:         strings.TrimSpace(opts.PostLoginRedirectURL),
		googleClientID:               strings.TrimSpace(opts.GoogleClientID),
		googleClientSecret:           strings.TrimSpace(opts.GoogleClientSecret),
		googleRedirectURL:            strings.TrimSpace(opts.GoogleRedirectURL),
		googleHostedDomain:           strings.TrimSpace(opts.GoogleHostedDomain),
		protector:                    protector,
		httpClient: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

/* ============================================================================

   JWT

============================================================================ */

func (h *AuthHandler) generateToken(user *models.User, rememberMe bool, authMethod string) (string, error) {
	if len(h.jwtSecret) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}

	idleTTL := h.sessionIdleTimeout
	expiresAt := time.Now().Add(h.accessTokenTTL)
	if rememberMe {
		idleTTL = h.rememberedSessionIdleTimeout
		expiresAt = time.Now().Add(h.rememberMeTTL)
	}

	claims := middleware.AccessClaims{
		UserID:                    user.ID,
		Email:                     user.Email,
		Role:                      user.Role,
		RememberMe:                rememberMe,
		SessionIdleTimeoutSeconds: int64(idleTTL / time.Second),
		AuthMethod:                strings.TrimSpace(authMethod),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

/* ============================================================================

   Cookies

============================================================================ */

func (h *AuthHandler) cookieSameSite() http.SameSite {
	// Cross-site requests from admin/FE -> api.* require SameSite=None + Secure=true
	if h.secure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (h *AuthHandler) cookieSecure() bool {
	// SameSite=None requires Secure=true in modern browsers.
	if h.secure {
		return true
	}
	return false
}

func (h *AuthHandler) setAuthCookie(c *gin.Context, token string, rememberMe bool) {
	maxAge := 0
	expires := time.Time{}
	idleTTL := h.sessionIdleTimeout
	activityMaxAge := 0
	activityExpires := time.Time{}

	if rememberMe {
		maxAge = int(h.rememberMeTTL / time.Second)
		expires = time.Now().Add(h.rememberMeTTL)
		idleTTL = h.rememberedSessionIdleTimeout
		activityMaxAge = int(idleTTL / time.Second)
		activityExpires = time.Now().Add(idleTTL)
	}

	sameSite := h.cookieSameSite()
	secure := h.cookieSecure()

	// Domain left empty => host-only cookie (api.wisdomchurchhq.org)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		Domain:   "",
		MaxAge:   maxAge,
		Expires:  expires,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})

	// Track inactivity (30 minutes)
	now := time.Now()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "last_activity",
		Value:    now.UTC().Format(time.RFC3339),
		Path:     "/",
		Domain:   "",
		MaxAge:   activityMaxAge,
		Expires:  activityExpires,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
}

func (h *AuthHandler) clearAuthCookie(c *gin.Context) {
	sameSite := h.cookieSameSite()
	secure := h.cookieSecure()

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		Domain:   "",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "last_activity",
		Value:    "",
		Path:     "/",
		Domain:   "",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
}

func (h *AuthHandler) loginMetadata(c *gin.Context) service.LoginMetadata {
	return service.LoginMetadata{
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		DeviceID: func() string {
			if v, err := c.Cookie("device_id"); err == nil {
				return v
			}
			return ""
		}(),
	}
}

func authUserPayload(user *models.User) gin.H {
	if user == nil {
		return gin.H{}
	}

	return gin.H{
		"id":                   user.ID,
		"first_name":           user.FirstName,
		"last_name":            user.LastName,
		"email":                user.Email,
		"role":                 user.Role,
		"is_active":            user.IsActive,
		"created_at":           user.CreatedAt,
		"updated_at":           user.UpdatedAt,
		"last_login_at":        user.LastLoginAt,
		"preferred_mfa_method": user.PreferredMFAMethod,
		"totp_enabled":         user.TOTPEnabled,
		"federated_provider":   user.FederatedProvider,
		"federated_linked_at":  user.FederatedLinkedAt,
	}
}

func (h *AuthHandler) issueAuthenticatedSession(c *gin.Context, user *models.User, rememberMe bool, authMethod string) error {
	token, err := h.generateToken(user, rememberMe, authMethod)
	if err != nil {
		return err
	}

	h.setAuthCookie(c, token, rememberMe)
	return nil
}

func (h *AuthHandler) effectivePostLoginRedirectURL() string {
	return strings.TrimSpace(h.postLoginRedirectURL)
}

/* ============================================================================

   Handlers

============================================================================ */

// Login establishes cookie-based session ONLY here
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required,email"`
		Password   string `json:"password" binding:"required,min=8"`
		RememberMe bool   `json:"rememberMe"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	req.Email = validation.NormalizeEmail(req.Email)
	req.Password = strings.TrimSpace(req.Password)

	meta := h.loginMetadata(c)

	result, err := h.service.Login(req.Email, req.Password, meta)
	if err != nil {
		status := http.StatusUnauthorized

		if errors.Is(err, service.ErrAdminPending) {
			status = http.StatusForbidden
		} else if errors.Is(err, service.ErrUserNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrWrongPassword) {
			status = http.StatusUnauthorized
		}

		utils.ErrorResponse(c, status, err.Error())
		return
	}

	if result.OTPRequired {
		utils.SuccessResponse(c, http.StatusAccepted, "Additional verification required", gin.H{
			"otp_required": true,
			"mfa_method":   result.MFAMethod,
			"purpose":      result.OTPPurpose,
			"expires_at":   result.OTPExpiresAt,
			"action_url":   result.OTPActionURL,
			"email":        req.Email,
		})
		return
	}

	if err := h.issueAuthenticatedSession(c, result.User, req.RememberMe, result.AuthMethod); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"user": authUserPayload(result.User),
		},
	})
}

// Register creates account but does NOT authenticate or set cookies
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		FirstName string `json:"first_name" binding:"required,min=2,max=50"`
		LastName  string `json:"last_name" binding:"required,min=2,max=50"`
		Email     string `json:"email" binding:"required,email"`
		Password  string `json:"password" binding:"required,min=8"`
		Role      string `json:"role" binding:"required,oneof=admin super_admin"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	req.Email = validation.NormalizeEmail(req.Email)
	req.FirstName = validation.NormalizeString(req.FirstName)
	req.LastName = validation.NormalizeString(req.LastName)
	req.Password = strings.TrimSpace(req.Password)

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
			"user": authUserPayload(user),
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
		"data":    authUserPayload(user),
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

	if !validation.BindJSON(c, &req) {
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
		"data":    authUserPayload(user),
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
		CurrentPassword string `json:"currentPassword" binding:"required,min=8"`
		NewPassword     string `json:"newPassword" binding:"required,min=8"`
		ConfirmPassword string `json:"confirmPassword"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	req.CurrentPassword = strings.TrimSpace(req.CurrentPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)

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

	rememberMe, _ := c.Get("remember_me")
	remembered := rememberMe == true

	authMethod := ""
	if rawAuthMethod, exists := c.Get("auth_method"); exists {
		if method, ok := rawAuthMethod.(string); ok {
			authMethod = strings.TrimSpace(method)
		}
	}
	if authMethod == "" {
		authMethod = "session_refresh"
	}

	if err := h.issueAuthenticatedSession(c, user, remembered, authMethod); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

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
		// Try unverified parse first
		if t, _, err := new(jwt.Parser).ParseUnverified(tokenCookie, jwt.MapClaims{}); err == nil {
			if claims, ok := t.Claims.(jwt.MapClaims); ok {
				if uid, ok := claims["user_id"].(string); ok {
					userID = uid
				}
			}
		} else {
			// Fallback: parse with verification
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

/* ============================================================================

   Password reset

============================================================================ */

func (h *AuthHandler) RequestPasswordReset(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	req.Email = validation.NormalizeEmail(req.Email)

	// Do NOT leak if the user exists. For ErrUserNotFound we still return 200.
	resp, err := h.service.RequestPasswordReset(req.Email, "")
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			utils.SuccessResponse(c, http.StatusOK,
				"If an account exists for that email, a password reset email has been sent.",
				nil,
			)
			return
		}

		// Any other error is internal (email provider / DB / etc.)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to start password reset process")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password reset email sent", resp)
}

func (h *AuthHandler) ConfirmPasswordReset(c *gin.Context) {
	var req struct {
		Email           string `json:"email" binding:"required,email"`
		Code            string `json:"code" binding:"required,len=6"`
		Purpose         string `json:"purpose" binding:"required"`
		NewPassword     string `json:"newPassword" binding:"required,min=8"`
		ConfirmPassword string `json:"confirmPassword"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	req.Email = validation.NormalizeEmail(req.Email)
	req.NewPassword = strings.TrimSpace(req.NewPassword)

	if req.ConfirmPassword != "" && req.NewPassword != req.ConfirmPassword {
		utils.ErrorResponse(c, http.StatusBadRequest, "New passwords do not match")
		return
	}

	if err := h.service.ResetPasswordWithOTP(req.Email, req.Code, req.Purpose, req.NewPassword); err != nil {
		// You can specialize error handling here if your service exposes
		// ErrOTPInvalid, ErrOTPExpired, etc. For now, we keep it generic.
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Password reset successful", nil)
}

/* ============================================================================

   OTP-based login

============================================================================ */

func (h *AuthHandler) VerifyLoginOTP(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required,email"`
		Code       string `json:"code" binding:"required,len=6"`
		Purpose    string `json:"purpose"`
		Method     string `json:"method"`
		RememberMe bool   `json:"rememberMe"`
	}

	if !validation.BindJSON(c, &req) {
		return
	}

	req.Email = validation.NormalizeEmail(req.Email)

	user, authMethod, err := h.service.VerifyLoginMFA(req.Email, req.Code, req.Purpose, req.Method, h.loginMetadata(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.issueAuthenticatedSession(c, user, req.RememberMe, authMethod); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login verified",
		"data": gin.H{
			"user": authUserPayload(user),
		},
	})
}

// ResendLoginOTP issues a fresh OTP for a recent login attempt.
func (h *AuthHandler) ResendLoginOTP(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}

	req.Email = validation.NormalizeEmail(req.Email)

	result, err := h.service.ResendLoginOTP(req.Email, h.loginMetadata(c))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrUserNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrAdminPending) {
			status = http.StatusForbidden
		}
		utils.ErrorResponse(c, status, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "OTP resent", gin.H{
		"otp_required": true,
		"mfa_method":   result.MFAMethod,
		"purpose":      result.OTPPurpose,
		"expires_at":   result.OTPExpiresAt,
		"action_url":   result.OTPActionURL,
		"email":        req.Email,
	})
}

/* ============================================================================

   MFA Settings

============================================================================ */

func (h *AuthHandler) GetMFASecurityProfile(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	profile, err := h.service.GetSecurityProfile(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Security profile loaded", profile)
}

func (h *AuthHandler) BeginTOTPSetup(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	payload, err := h.service.BeginTOTPSetup(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Authenticator setup initialized", payload)
}

func (h *AuthHandler) EnableTOTP(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		Code string `json:"code" binding:"required,len=6"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}

	profile, err := h.service.EnableTOTP(userID, req.Code)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Authenticator app enabled", profile)
}

func (h *AuthHandler) DisableTOTP(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		Code string `json:"code" binding:"required,len=6"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}

	profile, err := h.service.DisableTOTP(userID, req.Code)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Authenticator app disabled", profile)
}

func (h *AuthHandler) SetPreferredMFAMethod(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	var req struct {
		Method string `json:"method" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}

	profile, err := h.service.SetPreferredMFAMethod(userID, req.Method)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "MFA preference updated", profile)
}

/* ============================================================================

   Google OAuth

============================================================================ */

type googleOAuthState struct {
	State      string    `json:"state"`
	RememberMe bool      `json:"rememberMe"`
	IssuedAt   time.Time `json:"issuedAt"`
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type googleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
}

func (h *AuthHandler) StartGoogleOAuth(c *gin.Context) {
	if !h.googleOAuthEnabled() {
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "Google sign-in is not configured")
		return
	}
	if h.protector == nil {
		utils.ErrorResponse(c, http.StatusServiceUnavailable, "Authentication security is not configured")
		return
	}

	stateValue, err := generateOAuthStateValue()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to start Google sign-in")
		return
	}

	payload := googleOAuthState{
		State:      stateValue,
		RememberMe: strings.EqualFold(strings.TrimSpace(c.Query("rememberMe")), "true"),
		IssuedAt:   time.Now().UTC(),
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to start Google sign-in")
		return
	}

	h.setOAuthStateCookie(c, h.protector.SignPayload(raw))

	query := url.Values{}
	query.Set("client_id", h.googleClientID)
	query.Set("redirect_uri", h.googleRedirectURL)
	query.Set("response_type", "code")
	query.Set("scope", "openid email profile")
	query.Set("state", stateValue)
	query.Set("prompt", "select_account")
	if h.googleHostedDomain != "" {
		query.Set("hd", h.googleHostedDomain)
	}

	c.Redirect(http.StatusFound, "https://accounts.google.com/o/oauth2/v2/auth?"+query.Encode())
}

func (h *AuthHandler) HandleGoogleOAuthCallback(c *gin.Context) {
	if !h.googleOAuthEnabled() {
		h.renderOAuthError(c, http.StatusServiceUnavailable, "Google sign-in is not configured")
		return
	}
	if h.protector == nil {
		h.renderOAuthError(c, http.StatusServiceUnavailable, "Authentication security is not configured")
		return
	}
	if errParam := strings.TrimSpace(c.Query("error")); errParam != "" {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in was cancelled or denied")
		return
	}

	state, err := h.readOAuthState(c)
	if err != nil {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in session is invalid or expired")
		return
	}
	if strings.TrimSpace(c.Query("state")) != state.State {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in validation failed")
		return
	}

	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		h.clearOAuthStateCookie(c)
		h.renderOAuthError(c, http.StatusBadRequest, "Google sign-in did not return an authorization code")
		return
	}

	userInfo, err := h.fetchGoogleUserInfo(code)
	h.clearOAuthStateCookie(c)
	if err != nil {
		h.renderOAuthError(c, http.StatusBadGateway, "Failed to verify the Google account")
		return
	}
	if !userInfo.EmailVerified {
		h.renderOAuthError(c, http.StatusForbidden, "Google email address is not verified")
		return
	}

	result, err := h.service.CompleteGoogleLogin(
		userInfo.Sub,
		userInfo.Email,
		userInfo.GivenName,
		userInfo.FamilyName,
		h.loginMetadata(c),
	)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrAdminPending) {
			status = http.StatusForbidden
		}
		h.renderOAuthError(c, status, err.Error())
		return
	}

	if result.OTPRequired {
		h.renderOAuthMFAPage(c, result, state.RememberMe)
		return
	}

	if err := h.issueAuthenticatedSession(c, result.User, state.RememberMe, result.AuthMethod); err != nil {
		h.renderOAuthError(c, http.StatusInternalServerError, "Failed to create sign-in session")
		return
	}

	redirectURL := h.effectivePostLoginRedirectURL()
	if redirectURL != "" {
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	h.renderOAuthSuccess(c, "Google sign-in completed successfully.")
}

func (h *AuthHandler) googleOAuthEnabled() bool {
	return h.googleClientID != "" && h.googleClientSecret != "" && h.googleRedirectURL != ""
}

func (h *AuthHandler) setOAuthStateCookie(c *gin.Context, value string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "oauth_google_state",
		Value:    value,
		Path:     "/",
		MaxAge:   int((10 * time.Minute) / time.Second),
		Expires:  time.Now().Add(10 * time.Minute),
		Secure:   h.cookieSecure(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearOAuthStateCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "oauth_google_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   h.cookieSecure(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) readOAuthState(c *gin.Context) (*googleOAuthState, error) {
	cookie, err := c.Request.Cookie("oauth_google_state")
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, errors.New("missing oauth state")
	}

	payload, err := h.protector.VerifySignedPayload(cookie.Value)
	if err != nil {
		return nil, err
	}

	var state googleOAuthState
	if err := json.Unmarshal(payload, &state); err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.State) == "" || time.Since(state.IssuedAt) > 10*time.Minute {
		return nil, errors.New("oauth state expired")
	}

	return &state, nil
}

func (h *AuthHandler) fetchGoogleUserInfo(code string) (*googleUserInfo, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", h.googleClientID)
	form.Set("client_secret", h.googleClientSecret)
	form.Set("redirect_uri", h.googleRedirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequest(http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("google token exchange failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenPayload googleTokenResponse
	if err := json.Unmarshal(body, &tokenPayload); err != nil {
		return nil, err
	}
	if strings.TrimSpace(tokenPayload.AccessToken) == "" {
		return nil, errors.New("google access token missing")
	}

	userReq, err := http.NewRequest(http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenPayload.AccessToken)

	userResp, err := h.httpClient.Do(userReq)
	if err != nil {
		return nil, err
	}
	defer userResp.Body.Close()

	if userResp.StatusCode < 200 || userResp.StatusCode >= 300 {
		return nil, errors.New("google user info request failed")
	}

	userBody, err := io.ReadAll(userResp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo googleUserInfo
	if err := json.Unmarshal(userBody, &userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

func (h *AuthHandler) renderOAuthMFAPage(c *gin.Context, result *service.LoginResult, rememberMe bool) {
	if result == nil || result.User == nil {
		h.renderOAuthError(c, http.StatusBadRequest, "Additional verification could not be started")
		return
	}

	redirectURL := h.effectivePostLoginRedirectURL()
	methodLabel := "verification code"
	instructions := "Enter the code sent to your email to complete sign-in."
	if strings.TrimSpace(result.MFAMethod) == "totp" {
		methodLabel = "authenticator code"
		instructions = "Open your authenticator app and enter the current 6-digit code."
	}

	page := oauthMFAPageTemplate{
		Title:        "Complete Sign-in",
		Instructions: instructions,
		Email:        result.User.Email,
		Purpose:      result.OTPPurpose,
		Method:       result.MFAMethod,
		RememberMe:   rememberMe,
		RedirectURL:  redirectURL,
		InputLabel:   methodLabel,
	}

	renderOAuthTemplate(c, http.StatusAccepted, oauthMFATemplate, page)
}

func (h *AuthHandler) renderOAuthSuccess(c *gin.Context, message string) {
	renderOAuthTemplate(c, http.StatusOK, oauthMessageTemplate, oauthMessagePage{
		Title:       "Sign-in Complete",
		Message:     message,
		RedirectURL: h.effectivePostLoginRedirectURL(),
		IsError:     false,
	})
}

func (h *AuthHandler) renderOAuthError(c *gin.Context, status int, message string) {
	renderOAuthTemplate(c, status, oauthMessageTemplate, oauthMessagePage{
		Title:       "Sign-in Unavailable",
		Message:     message,
		RedirectURL: h.effectivePostLoginRedirectURL(),
		IsError:     true,
	})
}

type oauthMessagePage struct {
	Title       string
	Message     string
	RedirectURL string
	IsError     bool
}

type oauthMFAPageTemplate struct {
	Title        string
	Instructions string
	Email        string
	Purpose      string
	Method       string
	InputLabel   string
	RememberMe   bool
	RedirectURL  string
}

func renderOAuthTemplate(c *gin.Context, status int, markup string, data any) {
	tpl := template.Must(template.New("oauth-page").Parse(markup))
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	_ = tpl.Execute(c.Writer, data)
}

const oauthMessageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    body { margin: 0; font-family: "Segoe UI", Tahoma, Arial, sans-serif; background: linear-gradient(180deg, #eef4fb 0%, #f7fafc 100%); color: #0f172a; }
    main { max-width: 760px; margin: 80px auto; padding: 32px; background: #fff; border: 1px solid #dbe3ef; border-radius: 24px; box-shadow: 0 18px 50px rgba(15,23,42,.08); }
    h1 { margin: 0 0 12px; font-size: 34px; }
    p { margin: 0; line-height: 1.7; color: #475569; }
    a { display: inline-block; margin-top: 20px; padding: 12px 18px; border-radius: 999px; text-decoration: none; background: {{if .IsError}}#ffffff{{else}}#0f4c81{{end}}; color: {{if .IsError}}#0f4c81{{else}}#ffffff{{end}}; border: 1px solid #0f4c81; font-weight: 700; }
  </style>
</head>
<body>
  <main>
    <h1>{{.Title}}</h1>
    <p>{{.Message}}</p>
    {{if .RedirectURL}}<a href="{{.RedirectURL}}">Continue</a>{{end}}
  </main>
</body>
</html>`

const oauthMFATemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    body { margin: 0; font-family: "Segoe UI", Tahoma, Arial, sans-serif; background: linear-gradient(180deg, #eef4fb 0%, #f7fafc 100%); color: #0f172a; }
    main { max-width: 760px; margin: 80px auto; padding: 32px; background: #fff; border: 1px solid #dbe3ef; border-radius: 24px; box-shadow: 0 18px 50px rgba(15,23,42,.08); }
    h1 { margin: 0 0 12px; font-size: 34px; }
    p { margin: 0 0 20px; line-height: 1.7; color: #475569; }
    label { display: block; margin-bottom: 8px; font-weight: 700; }
    input { width: 100%; box-sizing: border-box; border: 1px solid #cbd5e1; border-radius: 16px; padding: 14px 16px; font: inherit; }
    button { margin-top: 16px; border: 0; border-radius: 999px; padding: 14px 22px; background: #0f4c81; color: #fff; font: inherit; font-weight: 700; cursor: pointer; }
    .status { margin-top: 18px; padding: 14px 16px; border-radius: 16px; display: none; }
    .status.error { display: block; background: #fef3f2; color: #b42318; border: 1px solid #fecdca; }
    .status.success { display: block; background: #ecfdf3; color: #166534; border: 1px solid #abefc6; }
  </style>
</head>
<body>
  <main>
    <h1>{{.Title}}</h1>
    <p>{{.Instructions}}</p>
    <form id="mfa-form">
      <label for="code">{{.InputLabel}}</label>
      <input id="code" name="code" inputmode="numeric" autocomplete="one-time-code" maxlength="6" required>
      <button type="submit">Complete Sign-in</button>
    </form>
    <div id="status" class="status"></div>
  </main>
  <script>
    const form = document.getElementById('mfa-form');
    const status = document.getElementById('status');
    const redirectUrl = {{printf "%q" .RedirectURL}};
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      status.className = 'status';
      status.textContent = '';
      const code = (document.getElementById('code').value || '').trim();
      if (code.length !== 6) {
        status.className = 'status error';
        status.textContent = 'Enter the 6-digit verification code.';
        return;
      }
      const response = await fetch('/api/v1/auth/otp/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: {{printf "%q" .Email}},
          purpose: {{printf "%q" .Purpose}},
          method: {{printf "%q" .Method}},
          rememberMe: {{if .RememberMe}}true{{else}}false{{end}},
          code,
        }),
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) {
        status.className = 'status error';
        status.textContent = body.message || 'Unable to complete sign-in.';
        return;
      }
      status.className = 'status success';
      status.textContent = 'Sign-in completed successfully.';
      if (redirectUrl) {
        window.location.assign(redirectUrl);
      }
    });
  </script>
</body>
</html>`

func generateOAuthStateValue() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
