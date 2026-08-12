// internal/handlers/auth_handler.go
package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"wisdomHouse-backend/internal/authutil"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

// Cookie names — mechanics live in internal/authutil (see AuthHandler.cookies).
const (
	authTokenCookieName    = authutil.AuthTokenCookieName
	lastActivityCookieName = authutil.LastActivityCookieName
	deviceIDCookieName     = authutil.DeviceIDCookieName
	oauthStateCookieName   = authutil.OAuthStateCookieName
	refreshTokenCookieName = authutil.RefreshTokenCookieName
)

type AuthHandlerOptions struct {
	JWTSecret                    string
	RSAPrivateKey                *rsa.PrivateKey
	Secure                       bool
	AccessTokenTTL               time.Duration
	RefreshTokenTTL              time.Duration
	RememberMeTTL                time.Duration
	SessionIdleTimeout           time.Duration
	RememberedSessionIdleTimeout time.Duration
	PostLoginRedirectURL         string
	AuthSecretKey                string
	GoogleClientID               string
	GoogleClientSecret           string
	GoogleRedirectURL            string
	GoogleHostedDomain           string
	RefreshTokenRepo             repository.RefreshTokenRepository
	Blocklist                    *authutil.TokenBlocklist
	PasswordHashCost             int
}

type AuthHandler struct {
	service                      service.AuthService
	jwtSecret                    []byte
	rsaPrivateKey                *rsa.PrivateKey
	secure                       bool
	accessTokenTTL               time.Duration
	refreshTokenTTL              time.Duration
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
	refreshTokenRepo             repository.RefreshTokenRepository
	blocklist                    *authutil.TokenBlocklist
	passwordHashCost             int
	cookies                      *authutil.CookieJar
}

func NewAuthHandler(service service.AuthService, opts AuthHandlerOptions) *AuthHandler {
	if strings.TrimSpace(opts.JWTSecret) == "" {
		applog.L().Warn("JWT_SECRET not set")
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

	hashCost := opts.PasswordHashCost
	if hashCost <= 0 {
		hashCost = 12
	}

	refreshTTL := opts.RefreshTokenTTL
	if refreshTTL <= 0 {
		refreshTTL = 7 * 24 * time.Hour
	}

	return &AuthHandler{
		service:                      service,
		jwtSecret:                    []byte(opts.JWTSecret),
		rsaPrivateKey:                opts.RSAPrivateKey,
		secure:                       opts.Secure,
		accessTokenTTL:               opts.AccessTokenTTL,
		refreshTokenTTL:              refreshTTL,
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
		refreshTokenRepo: opts.RefreshTokenRepo,
		blocklist:        opts.Blocklist,
		passwordHashCost: hashCost,
		cookies: authutil.NewCookieJar(
			opts.Secure,
			opts.SessionIdleTimeout,
			opts.RememberedSessionIdleTimeout,
			opts.RememberMeTTL,
		),
	}
}

/* ============================================================================

   Role / access helpers

============================================================================ */

func normalizeAccessRole(role string) string {
	value := strings.ToLower(strings.TrimSpace(role))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func isAdminAccessRole(role string) bool {
	switch normalizeAccessRole(role) {
	case "admin", "super_admin", "superadmin":
		return true
	default:
		return false
	}
}

func isAccountDeactivatedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "deactivated") || strings.Contains(message, "inactive")
}

func isSafePasswordResetNoopError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, service.ErrUserNotFound) || errors.Is(err, service.ErrAdminPending) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "deactivated") ||
		strings.Contains(message, "inactive") ||
		strings.Contains(message, "approval pending") ||
		strings.Contains(message, "awaiting")
}

func (h *AuthHandler) writeNoActiveSession(
	c *gin.Context,
	message string,
	accessStatus string,
	accessCode string,
	nextStep string,
) {
	if strings.TrimSpace(message) == "" {
		message = "No active session"
	}
	if strings.TrimSpace(accessStatus) == "" {
		accessStatus = "login_required"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": message,
		"data":    nil,
		"meta": gin.H{
			"authenticated": false,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
		},
	})
}

func writeAuthBlockedResponse(c *gin.Context, status int, message string, code string, nextStep string) {
	if status <= 0 {
		status = http.StatusForbidden
	}
	if strings.TrimSpace(message) == "" {
		message = "Access blocked"
	}

	c.JSON(status, gin.H{
		"status":      "error",
		"message":     message,
		"code":        code,
		"access_code": code,
		"next_step":   nextStep,
	})
}

/* ============================================================================

   JWT

============================================================================ */

// generateJTI creates a cryptographically random JWT ID for token revocation.
func generateJTI() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *AuthHandler) generateToken(user *models.User, rememberMe bool, authMethod string) (string, error) {
	if h.rsaPrivateKey == nil && len(h.jwtSecret) == 0 {
		return "", fmt.Errorf("JWT signing key not configured")
	}
	if user == nil {
		return "", fmt.Errorf("user is required")
	}
	if !user.IsActive {
		return "", errors.New("account is deactivated")
	}
	if isAdminAccessRole(user.Role) && !user.AdminApproved {
		return "", service.ErrAdminPending
	}

	now := time.Now().UTC()
	idleTTL := h.sessionIdleTimeout
	expiresAt := now.Add(h.accessTokenTTL)

	if rememberMe {
		idleTTL = h.rememberedSessionIdleTimeout
		// When RS256 + refresh tokens, access token TTL stays short regardless.
		// For HS256 legacy mode, keep the old behavior of extending access token TTL.
		if h.rsaPrivateKey == nil {
			expiresAt = now.Add(h.rememberMeTTL)
		}
	}

	claims := middleware.AccessClaims{
		UserID:                    user.ID,
		Email:                     user.Email,
		Role:                      user.Role,
		RememberMe:                rememberMe,
		SessionIdleTimeoutSeconds: int64(idleTTL / time.Second),
		AuthMethod:                strings.TrimSpace(authMethod),
		JTI:                       generateJTI(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   user.ID,
			Issuer:    middleware.JWTIssuer,
			Audience:  []string{middleware.JWTAudience},
		},
	}

	if h.rsaPrivateKey != nil {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		return token.SignedString(h.rsaPrivateKey)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

// issueRefreshToken creates a new refresh token in the token family and sets
// the refresh_token HTTPOnly cookie. No-op when refreshTokenRepo is not configured.
func (h *AuthHandler) issueRefreshToken(c *gin.Context, user *models.User, familyID string, rememberMe bool, deviceID string) error {
	if h.refreshTokenRepo == nil {
		return nil
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generating refresh token: %w", err)
	}
	rawHex := hex.EncodeToString(raw)

	sum := sha256.Sum256([]byte(rawHex))
	hash := hex.EncodeToString(sum[:])

	ttl := h.refreshTokenTTL
	if rememberMe && h.rememberMeTTL > ttl {
		ttl = h.rememberMeTTL
	}

	rt := &models.RefreshToken{
		UserID:    user.ID,
		FamilyID:  familyID,
		TokenHash: hash,
		DeviceID:  deviceID,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	if err := h.refreshTokenRepo.Create(rt); err != nil {
		return fmt.Errorf("storing refresh token: %w", err)
	}

	h.cookies.SetRefreshToken(c.Writer, rawHex, ttl)
	return nil
}

// hashRefreshToken returns the SHA-256 hex of the raw token value.
func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

/* ============================================================================

   Cookies — mechanics live in internal/authutil.CookieJar (h.cookies); these
   are thin wrappers so the rest of this file doesn't need to change shape.

============================================================================ */

func configuredCSRFCookieName() string {
	name := strings.TrimSpace(os.Getenv("AUTH_CSRF_COOKIE_NAME"))
	if name == "" {
		return middleware.DefaultCSRFCookieName
	}

	return name
}

// latestCookieValue returns the last non-empty cookie value for a name.
//
// This protects the auth handler during cookie-domain migrations. Browsers may
// send duplicate cookies when older host/path variants still exist.
func latestCookieValue(c *gin.Context, name string) (string, error) {
	if c == nil {
		return "", http.ErrNoCookie
	}
	return authutil.LatestCookieValue(c.Request, name)
}

func (h *AuthHandler) setAuthCookie(c *gin.Context, token string, rememberMe bool) {
	h.cookies.SetAuth(c.Writer, token, rememberMe)
}

func (h *AuthHandler) clearAuthCookie(c *gin.Context) {
	h.cookies.ClearAuth(c.Writer, configuredCSRFCookieName())

	// Compatibility with your existing CSRF middleware helper.
	middleware.ClearCSRFCookie(c, h.cookies.Secure, "")
}

// clearAuthCookieAll also removes the refresh_token cookie.
func (h *AuthHandler) clearAuthCookieAll(c *gin.Context) {
	h.clearAuthCookie(c)
	h.cookies.ExpireRefreshToken(c.Writer)
}

func (h *AuthHandler) loginMetadata(c *gin.Context) service.LoginMetadata {
	deviceID := ""
	if value, err := latestCookieValue(c, deviceIDCookieName); err == nil {
		deviceID = value
	}

	return service.LoginMetadata{
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		DeviceID:  deviceID,
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
		"admin_approved":       user.AdminApproved,
		"created_at":           user.CreatedAt,
		"updated_at":           user.UpdatedAt,
		"last_login_at":        user.LastLoginAt,
		"preferred_mfa_method": user.PreferredMFAMethod,
		"totp_enabled":         user.TOTPEnabled,
		"federated_provider":   user.FederatedProvider,
		"federated_linked_at":  user.FederatedLinkedAt,
	}
}

func deriveAccessStatus(user *models.User, authMethod string) (string, string, string) {
	if user == nil {
		return "login_required", "", ""
	}

	role := normalizeAccessRole(user.Role)
	authMethod = strings.ToLower(strings.TrimSpace(authMethod))

	isAdmin := role == "admin" || role == "super_admin" || role == "superadmin"
	if !user.IsActive {
		return "blocked", "account_deactivated", "/login"
	}
	if isAdmin && !user.AdminApproved {
		return "approval_pending", "admin_approval_pending", "/pending-approval"
	}
	if !isAdmin {
		return "ok", "", ""
	}

	if !user.TOTPEnabled {
		return "mfa_required", "admin_mfa_required", "/mfa/setup"
	}

	if authMethod != "totp" {
		return "mfa_required", "admin_totp_session_required", "/verify-otp"
	}

	return "ok", "", ""
}

func (h *AuthHandler) issueAuthenticatedSession(c *gin.Context, user *models.User, rememberMe bool, authMethod string) error {
	if user == nil {
		return errors.New("user is required")
	}
	if !user.IsActive {
		h.clearAuthCookie(c)
		return errors.New("account is deactivated")
	}
	if isAdminAccessRole(user.Role) && !user.AdminApproved {
		h.clearAuthCookie(c)
		return service.ErrAdminPending
	}

	token, err := h.generateToken(user, rememberMe, authMethod)
	if err != nil {
		return err
	}
	h.setAuthCookie(c, token, rememberMe)

	// Issue a new refresh token family for every fresh login.
	familyID := generateJTI()
	deviceID := ""
	if v, err2 := latestCookieValue(c, deviceIDCookieName); err2 == nil {
		deviceID = v
	}
	if err := h.issueRefreshToken(c, user, familyID, rememberMe, deviceID); err != nil {
		// Non-fatal: access token already set, log and continue.
		_ = err
	}

	return nil
}

func (h *AuthHandler) effectivePostLoginRedirectURL() string {
	return strings.TrimSpace(h.postLoginRedirectURL)
}

/* ============================================================================

   Handlers

============================================================================ */

// Login establishes cookie-based session ONLY here.
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
			h.clearAuthCookie(c)
			writeAuthBlockedResponse(
				c,
				http.StatusForbidden,
				"Your admin account is awaiting super-admin approval.",
				"admin_approval_pending",
				"/pending-approval",
			)
			return
		} else if errors.Is(err, service.ErrUserNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrWrongPassword) {
			status = http.StatusUnauthorized
		} else if isAccountDeactivatedError(err) {
			h.clearAuthCookie(c)
			status = http.StatusForbidden
		}

		utils.ErrorResponse(c, status, err.Error())
		return
	}

	if result == nil || result.User == nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid login response")
		return
	}

	if isAdminAccessRole(result.User.Role) && !result.User.AdminApproved {
		h.clearAuthCookie(c)
		writeAuthBlockedResponse(
			c,
			http.StatusForbidden,
			"Your admin account is awaiting super-admin approval.",
			"admin_approval_pending",
			"/pending-approval",
		)
		return
	}

	if result.OTPRequired {
		nextStep := "/verify-otp"
		accessCode := ""
		if strings.EqualFold(strings.TrimSpace(result.MFAMethod), "totp") {
			accessCode = "admin_totp_session_required"
		}

		utils.SuccessResponse(c, http.StatusAccepted, "Additional verification required", gin.H{
			"otp_required": true,
			"mfa_method":   result.MFAMethod,
			"purpose":      result.OTPPurpose,
			"expires_at":   result.OTPExpiresAt,
			"action_url":   result.OTPActionURL,
			"access_code":  accessCode,
			"next_step":    nextStep,
			"email":        req.Email,
		})
		return
	}

	if err := h.issueAuthenticatedSession(c, result.User, req.RememberMe, result.AuthMethod); err != nil {
		if errors.Is(err, service.ErrAdminPending) {
			writeAuthBlockedResponse(c, http.StatusForbidden, "Your admin account is awaiting super-admin approval.", "admin_approval_pending", "/pending-approval")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	accessStatus, accessCode, nextStep := deriveAccessStatus(result.User, result.AuthMethod)
	responseData := authUserPayload(result.User)
	responseData["auth_method"] = strings.TrimSpace(result.AuthMethod)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"user": responseData,
		},
		"meta": gin.H{
			"authenticated": true,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
		},
	})
}

// Register creates an admin access request only. It never authenticates the user
// and it never allows public creation of a super-admin account.
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

	role := normalizeAccessRole(req.Role)
	if role == "super_admin" || role == "superadmin" {
		h.clearAuthCookie(c)
		writeAuthBlockedResponse(
			c,
			http.StatusForbidden,
			"Super-admin accounts cannot be created from public registration. Create or promote super-admins only through a trusted server-side process.",
			"super_admin_registration_blocked",
			"",
		)
		return
	}
	if role != "admin" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Only admin access requests can be created from this endpoint")
		return
	}

	userData, err := h.service.Register(req.FirstName, req.LastName, req.Email, req.Password, role)
	if err != nil {
		h.clearAuthCookie(c)
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	user, ok := userData.(*models.User)
	if !ok || user == nil {
		h.clearAuthCookie(c)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}

	// Safety: ensure registration never leaves the user authenticated.
	h.clearAuthCookie(c)

	statusCode := http.StatusCreated
	message := "Registration successful. Please log in."
	accessStatus, accessCode, nextStep := deriveAccessStatus(user, "")
	if isAdminAccessRole(user.Role) && !user.AdminApproved {
		statusCode = http.StatusAccepted
		message = "Admin access request submitted. A super-admin must approve this account before login."
		accessStatus = "approval_pending"
		accessCode = "admin_approval_pending"
		nextStep = "/pending-approval"
	}

	c.JSON(statusCode, gin.H{
		"status":  "success",
		"message": message,
		"data": gin.H{
			"user": authUserPayload(user),
		},
		"meta": gin.H{
			"authenticated": false,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
		},
	})
}

func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		h.writeNoActiveSession(c, "No active session", "login_required", "", "")
		return
	}

	id, ok := userID.(string)
	if !ok || strings.TrimSpace(id) == "" {
		h.clearAuthCookie(c)
		h.writeNoActiveSession(c, "Invalid session", "login_required", "invalid_session", "/login")
		return
	}

	userData, err := h.service.GetUserByID(id)
	if err != nil {
		h.clearAuthCookie(c)
		if isAccountDeactivatedError(err) {
			h.writeNoActiveSession(c, "Account is deactivated", "blocked", "account_deactivated", "/login")
			return
		}
		h.writeNoActiveSession(c, "User not found", "login_required", "session_user_not_found", "/login")
		return
	}

	user, ok := userData.(*models.User)
	if !ok || user == nil {
		h.clearAuthCookie(c)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Invalid user data")
		return
	}

	authMethod := ""
	if rawAuthMethod, ok := c.Get("auth_method"); ok {
		if method, ok := rawAuthMethod.(string); ok {
			authMethod = method
		}
	}

	accessStatus, accessCode, nextStep := deriveAccessStatus(user, authMethod)
	if accessCode == "admin_approval_pending" || accessCode == "account_deactivated" {
		h.clearAuthCookie(c)
		h.writeNoActiveSession(c, "Session blocked", accessStatus, accessCode, nextStep)
		return
	}

	responseData := authUserPayload(user)
	responseData["auth_method"] = strings.TrimSpace(authMethod)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User retrieved successfully",
		"data":    responseData,
		"meta": gin.H{
			"authenticated": true,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
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

// RotateRefreshToken implements proper refresh token rotation (token family pattern).
// Called from an unprotected route — works even when the access token has expired.
// The refresh_token HTTPOnly cookie is the credential; the access token is NOT required.
func (h *AuthHandler) RotateRefreshToken(c *gin.Context) {
	if h.refreshTokenRepo == nil {
		utils.ErrorResponse(c, http.StatusNotImplemented, "Refresh token rotation not configured")
		return
	}

	rawRT, err := latestCookieValue(c, refreshTokenCookieName)
	if err != nil || strings.TrimSpace(rawRT) == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "No refresh token provided")
		return
	}

	hash := hashRefreshToken(rawRT)
	rt, err := h.refreshTokenRepo.FindByHash(hash)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to look up refresh token")
		return
	}

	// Token not found or already expired.
	if rt == nil || time.Now().UTC().After(rt.ExpiresAt) {
		h.clearAuthCookieAll(c)
		utils.ErrorResponse(c, http.StatusUnauthorized, "Refresh token expired or not found")
		return
	}

	// Token already revoked — this is a replay attack; revoke the whole family.
	if rt.RevokedAt != nil {
		_ = h.refreshTokenRepo.RevokeFamily(rt.FamilyID)
		h.clearAuthCookieAll(c)
		utils.ErrorResponse(c, http.StatusUnauthorized, "Refresh token already used — session invalidated for security")
		return
	}

	// Revoke the consumed token before issuing a new one.
	if err := h.refreshTokenRepo.RevokeByID(rt.ID); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to rotate refresh token")
		return
	}

	// Load the user.
	userData, err := h.service.GetUserByID(rt.UserID)
	if err != nil {
		h.clearAuthCookieAll(c)
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not found")
		return
	}
	user, ok := userData.(*models.User)
	if !ok || user == nil || !user.IsActive {
		h.clearAuthCookieAll(c)
		utils.ErrorResponse(c, http.StatusForbidden, "Account is inactive")
		return
	}

	// Issue new access token + new refresh token in the same family.
	rememberMe := rt.ExpiresAt.Sub(time.Now().UTC()) > h.refreshTokenTTL/2
	if err := h.issueAuthenticatedSessionWithFamily(c, user, rt.FamilyID, rememberMe, "token_rotation"); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to issue new tokens")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Token rotated successfully",
		"data":    gin.H{"user": authUserPayload(user)},
	})
}

// issueAuthenticatedSessionWithFamily issues new access + refresh tokens reusing
// an existing family ID (for rotation — keeps the family chain intact).
func (h *AuthHandler) issueAuthenticatedSessionWithFamily(c *gin.Context, user *models.User, familyID string, rememberMe bool, authMethod string) error {
	token, err := h.generateToken(user, rememberMe, authMethod)
	if err != nil {
		return err
	}
	h.setAuthCookie(c, token, rememberMe)

	deviceID := ""
	if v, err2 := latestCookieValue(c, deviceIDCookieName); err2 == nil {
		deviceID = v
	}
	return h.issueRefreshToken(c, user, familyID, rememberMe, deviceID)
}

// Keep the existing RefreshToken handler (requires valid access token — used for
// "extend session while already logged in" without consuming a refresh token).
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	id, ok := userID.(string)
	if !ok || strings.TrimSpace(id) == "" {
		h.clearAuthCookie(c)
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid session")
		return
	}

	userData, err := h.service.GetUserByID(id)
	if err != nil {
		h.clearAuthCookie(c)
		utils.ErrorResponse(c, http.StatusNotFound, "User not found")
		return
	}

	user, ok := userData.(*models.User)
	if !ok || user == nil {
		h.clearAuthCookie(c)
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

	accessStatus, accessCode, nextStep := deriveAccessStatus(user, authMethod)
	if accessCode == "admin_approval_pending" || accessCode == "account_deactivated" {
		h.clearAuthCookie(c)
		writeAuthBlockedResponse(c, http.StatusForbidden, "Session is no longer permitted.", accessCode, nextStep)
		return
	}

	// Never downgrade a TOTP-verified admin session to a generic refresh method.
	// If the auth method is missing for an admin, force the user through login/MFA again.
	if isAdminAccessRole(user.Role) && accessStatus != "ok" {
		utils.ErrorResponse(c, http.StatusForbidden, accessCode)
		return
	}
	if strings.TrimSpace(authMethod) == "" {
		authMethod = "session_refresh"
	}

	if err := h.issueAuthenticatedSession(c, user, remembered, authMethod); err != nil {
		if errors.Is(err, service.ErrAdminPending) {
			writeAuthBlockedResponse(c, http.StatusForbidden, "Your admin account is awaiting super-admin approval.", "admin_approval_pending", "/pending-approval")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	responseData := authUserPayload(user)
	responseData["auth_method"] = strings.TrimSpace(authMethod)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Token refreshed successfully",
		"data": gin.H{
			"user": responseData,
		},
		"meta": gin.H{
			"authenticated": true,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
		},
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	deviceID := ""
	if value, err := latestCookieValue(c, deviceIDCookieName); err == nil {
		deviceID = value
	}

	// Extract claims without full verification so logout always works even if
	// the access token is expired or the signing secret was rotated.
	userID, jti := "", ""
	if tokenCookie, err := latestCookieValue(c, authTokenCookieName); err == nil && tokenCookie != "" {
		if t, _, err := new(jwt.Parser).ParseUnverified(tokenCookie, jwt.MapClaims{}); err == nil {
			if claims, ok := t.Claims.(jwt.MapClaims); ok {
				if uid, ok := claims["user_id"].(string); ok {
					userID = uid
				}
				if j, ok := claims["jti"].(string); ok {
					jti = j
				}
			}
		}
	}

	// Block the access token JTI so it cannot be reused before natural expiry.
	if h.blocklist != nil && jti != "" {
		// Use the configured access TTL as upper bound for the blocklist entry.
		_ = h.blocklist.Block(ctx, jti, h.accessTokenTTL)
	}

	// Revoke all refresh tokens for the device (or all for the user on full logout).
	if h.refreshTokenRepo != nil {
		if rawRT, err := latestCookieValue(c, refreshTokenCookieName); err == nil && rawRT != "" {
			hash := hashRefreshToken(rawRT)
			if rt, err := h.refreshTokenRepo.FindByHash(hash); err == nil && rt != nil {
				_ = h.refreshTokenRepo.RevokeFamily(rt.FamilyID)
			}
		}
	}

	if userID != "" && deviceID != "" {
		_ = h.service.UntrustDevice(userID, deviceID)
	}

	h.clearAuthCookieAll(c)

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

	// Do NOT leak if the user exists, is pending approval, or is inactive.
	_, err := h.service.RequestPasswordReset(req.Email, "")
	if err != nil {
		if isSafePasswordResetNoopError(err) {
			utils.SuccessResponse(c, http.StatusOK,
				"If an account exists for that email, a password reset email has been sent.",
				nil,
			)
			return
		}

		// Any other error is internal (email provider / DB / etc.) — log without exposing email
		applog.L().Warn("password reset start failed", "error", err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to start password reset process")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "If an account exists for that email, a password reset email has been sent.", nil)
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

	if user == nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid verification response")
		return
	}
	if isAdminAccessRole(user.Role) && !user.AdminApproved {
		h.clearAuthCookie(c)
		writeAuthBlockedResponse(c, http.StatusForbidden, "Your admin account is awaiting super-admin approval.", "admin_approval_pending", "/pending-approval")
		return
	}

	if err := h.issueAuthenticatedSession(c, user, req.RememberMe, authMethod); err != nil {
		if errors.Is(err, service.ErrAdminPending) {
			writeAuthBlockedResponse(c, http.StatusForbidden, "Your admin account is awaiting super-admin approval.", "admin_approval_pending", "/pending-approval")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	accessStatus, accessCode, nextStep := deriveAccessStatus(user, authMethod)
	responseData := authUserPayload(user)
	responseData["auth_method"] = strings.TrimSpace(authMethod)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login verified",
		"data": gin.H{
			"user": responseData,
		},
		"meta": gin.H{
			"authenticated": true,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
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
			h.clearAuthCookie(c)
			writeAuthBlockedResponse(c, http.StatusForbidden, "Your admin account is awaiting super-admin approval.", "admin_approval_pending", "/pending-approval")
			return
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

func (h *AuthHandler) GetCSRFToken(c *gin.Context) {
	token, _ := c.Get("csrf_token")
	tokenStr, _ := token.(string)
	headerName, _ := c.Get("csrf_header")
	headerStr, _ := headerName.(string)

	if strings.TrimSpace(headerStr) == "" {
		headerStr = middleware.DefaultCSRFHeaderName
	}

	if strings.TrimSpace(tokenStr) == "" {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to issue CSRF token")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "CSRF token issued", gin.H{
		"token":  tokenStr,
		"header": headerStr,
	})
}
