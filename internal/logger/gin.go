package logger

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GinMiddleware returns a Gin middleware that:
//  1. Creates a request-scoped logger enriched with "request_id".
//  2. Stores it in the request context so handlers can retrieve it via FromGin.
//  3. After the handler chain completes, logs the request summary at INFO
//     (ERROR for 5xx responses).
//
// It must be registered AFTER the RequestID middleware so that request_id is
// already present in the Gin context.
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		reqID, _ := c.Get("request_id")
		reqLogger := L().With("request_id", reqID)

		// Inject into request context so service/repo layers can pick it up.
		c.Request = c.Request.WithContext(WithContext(c.Request.Context(), reqLogger))

		c.Next()

		path := c.Request.URL.Path
		if raw := c.Request.URL.RawQuery; raw != "" {
			path = path + "?" + raw
		}

		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", reqID,
		}

		if status >= 500 {
			reqLogger.Error("request completed", attrs...)
		} else {
			reqLogger.Info("request completed", attrs...)
		}
	}
}

// FromGin is a convenience wrapper that retrieves the request-scoped logger
// from a Gin context. Falls back to the global logger if none was stored.
func FromGin(c *gin.Context) *slog.Logger {
	return FromContext(c.Request.Context())
}

// AuditMiddleware replaces middleware.AuditLogger — logs mutating requests
// with structured fields including the authenticated user's ID and role.
func AuditMiddleware(scope string) gin.HandlerFunc {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "api"
	}
	return func(c *gin.Context) {
		method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
		switch method {
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

		FromContext(c.Request.Context()).Info("audit",
			"scope", scope,
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"user_id", userID,
			"role", role,
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		)
	}
}
