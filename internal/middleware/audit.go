package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

// AuditLogger returns a Gin middleware that records mutating requests (POST,
// PUT, PATCH, DELETE): always as a structured log line, and — when repo is
// non-nil — also as a durable row in audit_logs, so admin screens (recent
// activity, audit log views) have real data to show instead of only log
// lines nobody queries. The DB write happens in a detached goroutine so a
// storage hiccup never adds latency to, or fails, the actual request.
func AuditLogger(scope string, repo repository.AuditLogRepository) gin.HandlerFunc {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "api"
	}

	return func(c *gin.Context) {
		switch strings.ToUpper(strings.TrimSpace(c.Request.Method)) {
		case "POST", "PUT", "PATCH", "DELETE":
		default:
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		requestID, _ := c.Get("request_id")
		userID, _ := c.Get("user_id")
		role, _ := c.Get("role")
		status := c.Writer.Status()
		latency := time.Since(start)
		method := c.Request.Method
		path := c.Request.URL.Path
		ip := c.ClientIP()
		userAgent := c.Request.UserAgent()
		requestIDStr, _ := requestID.(string)
		roleStr, _ := role.(string)

		logger.FromContext(c.Request.Context()).Info("audit",
			"scope", scope,
			"request_id", requestID,
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"user_id", userID,
			"role", role,
			"ip", ip,
			"user_agent", userAgent,
		)

		if repo == nil {
			return
		}

		entry := &models.AuditLog{
			Scope:      scope,
			Method:     method,
			Path:       path,
			StatusCode: status,
			LatencyMS:  latency.Milliseconds(),
			Role:       roleStr,
			IP:         ip,
			UserAgent:  userAgent,
			RequestID:  requestIDStr,
			CreatedAt:  start.UTC(),
		}
		if userIDStr, ok := userID.(string); ok && userIDStr != "" {
			entry.UserID = &userIDStr
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := repo.Create(ctx, entry); err != nil {
				logger.L().Warn("audit log persist failed", "scope", scope, "error", err)
			}
		}()
	}
}
