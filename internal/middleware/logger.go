package middleware

import (
	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/logger"
)

// Logger returns a Gin request-logging middleware backed by log/slog.
// The level parameter is accepted for backward-compatibility but filtering
// is now controlled by logger.Init(level, env) called at startup.
func Logger(_ string) gin.HandlerFunc {
	return logger.GinMiddleware()
}
