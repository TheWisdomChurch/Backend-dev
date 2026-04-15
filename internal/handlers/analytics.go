package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AnalyticsHandler struct {
	db             *database.Database
	decisionEngine service.DecisionSupportService
}

func NewAnalyticsHandler(db *database.Database, redisCache *cache.RedisClient) *AnalyticsHandler {
	return &AnalyticsHandler{
		db:             db,
		decisionEngine: service.NewDecisionSupportService(db, redisCache),
	}
}

func (h *AnalyticsHandler) GetAdminAnalytics(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var totalEvents int64
	var upcomingEvents int64

	if err := h.db.WithContext(ctx).Model(&models.Event{}).Count(&totalEvents).Error; err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}

	if err := h.db.WithContext(ctx).
		Model(&models.Event{}).
		Where("status = ?", models.EventStatusUpcoming).
		Count(&upcomingEvents).Error; err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}

	type sumRow struct{ Sum int64 }
	var row sumRow
	if err := h.db.WithContext(ctx).
		Model(&models.Event{}).
		Select("COALESCE(SUM(attendees),0) as sum").
		Scan(&row).Error; err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}
	totalAttendees := row.Sum

	type catRow struct {
		Category string
		Count    int64
	}
	var cats []catRow
	if err := h.db.WithContext(ctx).
		Model(&models.Event{}).
		Select("category, COUNT(*) as count").
		Group("category").
		Scan(&cats).Error; err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}

	eventsByCategory := map[string]uint64{}
	for _, r := range cats {
		eventsByCategory[r.Category] = uint64(r.Count)
	}

	// Keep structure stable for frontend; you can implement real month grouping later.
	monthlyStats := []gin.H{}

	utils.SuccessResponse(c, http.StatusOK, "Analytics retrieved successfully", gin.H{
		"totalEvents":      totalEvents,
		"upcomingEvents":   upcomingEvents,
		"totalAttendees":   totalAttendees,
		"eventsByCategory": eventsByCategory,
		"monthlyStats":     monthlyStats,
	})
}

func (h *AnalyticsHandler) GetDecisionInsights(c *gin.Context) {
	if h.decisionEngine == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Decision insights engine is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	insights, err := h.decisionEngine.GetInsights(ctx)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute decision insights")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Decision insights retrieved successfully", insights)
}
