package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/database"
)

// HealthHandler serves the /healthz (liveness) and /readyz (readiness)
// endpoints. Both are unauthenticated.
type HealthHandler struct {
	db    *database.Database
	redis *cache.RedisClient // may be nil when Redis is not configured
}

func NewHealthHandler(db *database.Database, redis *cache.RedisClient) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Liveness returns 200 immediately. It only proves the process is alive and
// the HTTP stack is responding — no dependency checks.
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readiness pings Postgres and Redis (if configured). Returns 200 when all
// dependencies are healthy, 503 with per-component detail otherwise.
func (h *HealthHandler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	checks := gin.H{}
	healthy := true

	// Postgres check
	sqlDB, err := h.db.DB.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		checks["db"] = "error"
		healthy = false
	} else {
		checks["db"] = "ok"
	}

	// Redis check (optional)
	if h.redis != nil {
		if err := h.redis.Ping(ctx); err != nil {
			checks["redis"] = "error"
			healthy = false
		} else {
			checks["redis"] = "ok"
		}
	}

	status := http.StatusOK
	statusStr := "ok"
	if !healthy {
		status = http.StatusServiceUnavailable
		statusStr = "unavailable"
	}

	c.JSON(status, gin.H{"status": statusStr, "checks": checks})
}
