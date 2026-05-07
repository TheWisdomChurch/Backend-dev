package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/repository"
)

func normalizeAdminSecurityRole(role string) string {
	cleaned := strings.ToLower(strings.TrimSpace(role))
	cleaned = strings.ReplaceAll(cleaned, "-", "_")
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	return cleaned
}

func isAdminSecurityRole(role string) bool {
	role = normalizeAdminSecurityRole(role)
	return role == "admin" || role == "super_admin"
}

func adminSecurityError(
	c *gin.Context,
	status int,
	message string,
	code string,
	extra gin.H,
) {
	payload := gin.H{
		"error":      http.StatusText(status),
		"message":    message,
		"code":       code,
		"statusCode": status,
	}

	for key, value := range extra {
		payload[key] = value
	}

	c.JSON(status, payload)
	c.Abort()
}

// RequireApprovedAdmin re-checks the live database user before any admin route runs.
//
// This prevents stale JWT/cookie sessions from bypassing security after an admin
// account is disabled, downgraded, deleted, or moved back to pending approval.
func RequireApprovedAdmin(userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if userRepo == nil {
			adminSecurityError(
				c,
				http.StatusInternalServerError,
				"Admin security is not configured.",
				"admin_security_not_configured",
				nil,
			)
			return
		}

		roleValue, exists := c.Get("role")
		if !exists {
			adminSecurityError(
				c,
				http.StatusUnauthorized,
				"User role not found.",
				"session_role_missing",
				nil,
			)
			return
		}

		sessionRole, _ := roleValue.(string)
		sessionRole = normalizeAdminSecurityRole(sessionRole)

		if !isAdminSecurityRole(sessionRole) {
			c.Next()
			return
		}

		userID, ok := GetUserIDFromContext(c)
		if !ok {
			adminSecurityError(
				c,
				http.StatusUnauthorized,
				"Invalid session.",
				"invalid_session",
				nil,
			)
			return
		}

		user, err := userRepo.FindByID(userID)
		if err != nil || user == nil {
			adminSecurityError(
				c,
				http.StatusUnauthorized,
				"Invalid session.",
				"invalid_session",
				nil,
			)
			return
		}

		dbRole := normalizeAdminSecurityRole(user.Role)
		if !isAdminSecurityRole(dbRole) {
			adminSecurityError(
				c,
				http.StatusForbidden,
				"This account no longer has admin access.",
				"admin_access_revoked",
				gin.H{
					"requiredAction": "login_again",
				},
			)
			return
		}

		if dbRole != sessionRole {
			adminSecurityError(
				c,
				http.StatusForbidden,
				"Your session role is outdated. Please log in again.",
				"session_role_mismatch",
				gin.H{
					"requiredAction": "login_again",
				},
			)
			return
		}

		if !user.IsActive || !user.AdminApproved {
			adminSecurityError(
				c,
				http.StatusForbidden,
				"Your admin account is awaiting super-admin approval.",
				"admin_approval_required",
				gin.H{
					"approvalStatus": "pending",
					"requiredAction": "wait_for_super_admin_approval",
				},
			)
			return
		}

		// Refresh context from DB so downstream permission/role middleware uses
		// current account state, not only JWT claims.
		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
		c.Set("role", dbRole)
		c.Set("admin_db_role", dbRole)
		c.Set("admin_totp_enabled", user.TOTPEnabled)

		c.Next()
	}
}

// RequireAdminMFA enforces strict MFA for admin/super_admin routes.
//
// Policy:
//   - approved admins without TOTP enabled must go to /mfa/setup
//   - admins with TOTP enabled must use a TOTP-verified session
//   - email OTP alone is not enough for protected admin APIs once TOTP is enabled
func RequireAdminMFA(userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := ""

		if rawRole, exists := c.Get("admin_db_role"); exists {
			if value, ok := rawRole.(string); ok {
				role = normalizeAdminSecurityRole(value)
			}
		}

		if role == "" {
			if rawRole, exists := c.Get("role"); exists {
				if value, ok := rawRole.(string); ok {
					role = normalizeAdminSecurityRole(value)
				}
			}
		}

		if !isAdminSecurityRole(role) {
			c.Next()
			return
		}

		totpEnabled := false
		if rawTOTP, exists := c.Get("admin_totp_enabled"); exists {
			if value, ok := rawTOTP.(bool); ok {
				totpEnabled = value
			}
		} else {
			if userRepo == nil {
				adminSecurityError(
					c,
					http.StatusInternalServerError,
					"Admin MFA security is not configured.",
					"admin_mfa_not_configured",
					nil,
				)
				return
			}

			userID, ok := GetUserIDFromContext(c)
			if !ok {
				adminSecurityError(
					c,
					http.StatusUnauthorized,
					"Invalid session.",
					"invalid_session",
					nil,
				)
				return
			}

			user, err := userRepo.FindByID(userID)
			if err != nil || user == nil {
				adminSecurityError(
					c,
					http.StatusUnauthorized,
					"Invalid session.",
					"invalid_session",
					nil,
				)
				return
			}

			totpEnabled = user.TOTPEnabled
		}

		if !totpEnabled {
			adminSecurityError(
				c,
				http.StatusForbidden,
				"Admin MFA is required. Enable authenticator app verification to continue.",
				"admin_mfa_required",
				gin.H{
					"nextStep":        "/mfa/setup",
					"requiredMethod":  "totp",
					"requiredAction":  "enable_totp",
					"mfaRequired":     true,
					"totpEnabled":     false,
					"adminRouteBlock": true,
				},
			)
			return
		}

		authMethod := ""
		if rawMethod, exists := c.Get("auth_method"); exists {
			if value, ok := rawMethod.(string); ok {
				authMethod = strings.ToLower(strings.TrimSpace(value))
			}
		}

		if authMethod != "totp" {
			adminSecurityError(
				c,
				http.StatusForbidden,
				"Admin routes require a TOTP-verified session.",
				"admin_totp_session_required",
				gin.H{
					"nextStep":        "/verify-otp",
					"requiredMethod":  "totp",
					"requiredAction":  "verify_totp_session",
					"mfaRequired":     true,
					"totpEnabled":     true,
					"adminRouteBlock": true,
				},
			)
			return
		}

		c.Next()
	}
}