package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/repository"
)

// RequireApprovedAdmin re-validates elevated sessions against the database.
// It prevents stale JWT cookies from accessing admin routes after an account is
// deactivated, demoted, or still awaiting super-admin approval.
func RequireApprovedAdmin(userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleValue, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":      "Unauthorized",
				"message":    "User role not found",
				"statusCode": http.StatusUnauthorized,
			})
			c.Abort()
			return
		}

		role, _ := roleValue.(string)
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

		dbRole := normalizeRole(user.Role)
		if dbRole != "admin" && dbRole != "super_admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "This account no longer has admin access. Please log in again.",
				"code":       "admin_role_required",
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		if dbRole != role {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "Session role does not match the current account role. Please log in again.",
				"code":       "session_role_mismatch",
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		if !user.IsActive || !user.AdminApproved {
			c.JSON(http.StatusForbidden, gin.H{
				"error":          "Forbidden",
				"message":        "Your admin account is awaiting super-admin approval.",
				"code":           "admin_approval_required",
				"approvalStatus": "pending",
				"requiredAction": "wait_for_super_admin_approval",
				"statusCode":     http.StatusForbidden,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
