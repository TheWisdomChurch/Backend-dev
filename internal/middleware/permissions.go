package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Permission string

const (
	PermissionAdminAccess         Permission = "admin:access"
	PermissionAdminRead           Permission = "admin:read"
	PermissionAdminWrite          Permission = "admin:write"
	PermissionUsersManage         Permission = "admin:users:manage"
	PermissionSecurityRead        Permission = "admin:security:read"
	PermissionAnalyticsRead       Permission = "admin:analytics:read"
	PermissionContentManage       Permission = "admin:content:manage"
	PermissionEngagementRead      Permission = "admin:engagement:read"
	PermissionEngagementManage    Permission = "admin:engagement:manage"
	PermissionFormsRead           Permission = "admin:forms:read"
	PermissionFormsManage         Permission = "admin:forms:manage"
	PermissionFormsExport         Permission = "admin:forms:export"
	PermissionFormsCampaign       Permission = "admin:forms:campaign"
	PermissionNotificationsManage Permission = "admin:notifications:manage"
	PermissionApprovalsRead       Permission = "admin:approvals:read"
	PermissionEmailManage         Permission = "admin:email:manage"
	PermissionUploadsManage       Permission = "admin:uploads:manage"
	PermissionEventsManage        Permission = "admin:events:manage"
	PermissionReelsManage         Permission = "admin:reels:manage"
	PermissionWorkforceManage     Permission = "admin:workforce:manage"
	PermissionMembersManage       Permission = "admin:members:manage"
	PermissionStoreManage         Permission = "admin:store:manage"
	PermissionLeadershipManage    Permission = "admin:leadership:manage"
	// Sensitive pastoral content is denied to ordinary admins by default.
	// Super-admins inherit every permission through HasPermission.
	PermissionPrayerRequestsManage Permission = "admin:prayer-requests:manage"
)

var rolePermissions = map[string]map[Permission]struct{}{
	"admin": {
		PermissionAdminAccess:         {},
		PermissionAdminRead:           {},
		PermissionAdminWrite:          {},
		PermissionUsersManage:         {},
		PermissionSecurityRead:        {},
		PermissionAnalyticsRead:       {},
		PermissionContentManage:       {},
		PermissionEngagementRead:      {},
		PermissionEngagementManage:    {},
		PermissionFormsRead:           {},
		PermissionFormsManage:         {},
		PermissionFormsExport:         {},
		PermissionFormsCampaign:       {},
		PermissionNotificationsManage: {},
		PermissionApprovalsRead:       {},
		PermissionEmailManage:         {},
		PermissionUploadsManage:       {},
		PermissionEventsManage:        {},
		PermissionReelsManage:         {},
		PermissionWorkforceManage:     {},
		PermissionMembersManage:       {},
		PermissionStoreManage:         {},
		PermissionLeadershipManage:    {},
	},
	"super_admin": {
		PermissionAdminAccess:         {},
		PermissionAdminRead:           {},
		PermissionAdminWrite:          {},
		PermissionUsersManage:         {},
		PermissionSecurityRead:        {},
		PermissionAnalyticsRead:       {},
		PermissionContentManage:       {},
		PermissionEngagementRead:      {},
		PermissionEngagementManage:    {},
		PermissionFormsRead:           {},
		PermissionFormsManage:         {},
		PermissionFormsExport:         {},
		PermissionFormsCampaign:       {},
		PermissionNotificationsManage: {},
		PermissionApprovalsRead:       {},
		PermissionEmailManage:         {},
		PermissionUploadsManage:       {},
		PermissionEventsManage:        {},
		PermissionReelsManage:         {},
		PermissionWorkforceManage:     {},
		PermissionMembersManage:       {},
		PermissionStoreManage:         {},
		PermissionLeadershipManage:    {},
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
