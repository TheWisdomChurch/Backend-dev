package middleware

import (
	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/logger"
)

// AuditLogger returns a Gin middleware that logs mutating requests (POST, PUT,
// PATCH, DELETE) with structured fields via log/slog.
func AuditLogger(scope string) gin.HandlerFunc {
	return logger.AuditMiddleware(scope)
}

func isMutatingMethod(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
