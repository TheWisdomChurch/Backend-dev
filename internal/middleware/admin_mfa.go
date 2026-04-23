package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/repository"
)

// RequireAdminMFA enforces that admin/super_admin sessions are protected by TOTP MFA.
func RequireAdminMFA(userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleVal, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "Unauthorized",
				"message":    "User role not found",
				"statusCode": http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		role, _ := roleVal.(string)
		role = normalizeRole(role)
		if role != "admin" && role != "super_admin" {
			c.Next()
			return
		}

		userID, ok := GetUserIDFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "Unauthorized",
				"message":    "Invalid session",
				"statusCode": http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		user, err := userRepo.FindByID(userID)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "Unauthorized",
				"message":    "Invalid session",
				"statusCode": http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		if !user.TOTPEnabled {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "Admin MFA is required. Enable TOTP to continue.",
				"code":       "admin_mfa_required",
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		authMethod := ""
		if raw, exists := c.Get("auth_method"); exists {
			if method, ok := raw.(string); ok {
				authMethod = strings.ToLower(strings.TrimSpace(method))
			}
		}
		if authMethod != "totp" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":          "Forbidden",
				"message":        "Admin routes require a TOTP-verified session.",
				"code":           "admin_totp_session_required",
				"requiredMethod": "totp",
				"statusCode":     http.StatusForbidden,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
