package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Permission string

const (
	PermissionAdminAccess   Permission = "admin:access"
	PermissionAdminRead     Permission = "admin:read"
	PermissionAdminWrite    Permission = "admin:write"
	PermissionUsersManage   Permission = "admin:users:manage"
	PermissionSecurityRead  Permission = "admin:security:read"
	PermissionFormsExport   Permission = "admin:forms:export"
	PermissionFormsCampaign Permission = "admin:forms:campaign"
)

var rolePermissions = map[string]map[Permission]struct{}{
	"admin": {
		PermissionAdminAccess:   {},
		PermissionAdminRead:     {},
		PermissionAdminWrite:    {},
		PermissionUsersManage:   {},
		PermissionSecurityRead:  {},
		PermissionFormsExport:   {},
		PermissionFormsCampaign: {},
	},
	"super_admin": {
		PermissionAdminAccess:   {},
		PermissionAdminRead:     {},
		PermissionAdminWrite:    {},
		PermissionUsersManage:   {},
		PermissionSecurityRead:  {},
		PermissionFormsExport:   {},
		PermissionFormsCampaign: {},
	},
}

func RequirePermission(permission Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleRaw, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "User role not found",
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		role, ok := roleRaw.(string)
		if !ok || strings.TrimSpace(role) == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "Invalid role format",
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		if !HasPermission(role, permission) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":      "Forbidden",
				"message":    "Insufficient permissions",
				"permission": string(permission),
				"statusCode": http.StatusForbidden,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func HasPermission(role string, permission Permission) bool {
	role = normalizeRole(role)
	if role == "super_admin" {
		return true
	}

	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	_, exists := perms[permission]
	return exists
}

func normalizeRole(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
