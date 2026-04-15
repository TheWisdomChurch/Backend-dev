package handlers

import (
	"context"
	"encoding/json"
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

func (h *AnalyticsHandler) IngestEvents(c *gin.Context) {
	var req struct {
		BatchID string `json:"batchId"`
		Events  []struct {
			ID        string `json:"id"`
			Category  string `json:"category"`
			Action    string `json:"action"`
			Timestamp int64  `json:"timestamp"`
		} `json:"events"`
		Session struct {
			SessionID string `json:"sessionId"`
			UserID    string `json:"userId"`
		} `json:"session"`
		UserProfile struct {
			UserID string `json:"userId"`
		} `json:"userProfile"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid analytics payload")
		return
	}
	if req.BatchID == "" || len(req.Events) == 0 || req.Session.SessionID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "batchId, session.sessionId and events are required")
		return
	}

	userID := req.UserProfile.UserID
	if userID == "" {
		userID = req.Session.UserID
	}
	userIDPtr := func() *string {
		if userID == "" {
			return nil
		}
		out := userID
		return &out
	}()

	rawPayload, _ := json.Marshal(req)

	record := models.AnalyticsBatch{
		BatchID:    req.BatchID,
		SessionID:  req.Session.SessionID,
		UserID:     userIDPtr,
		EventCount: len(req.Events),
		Payload:    rawPayload,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := h.db.WithContext(ctx).Create(&record).Error; err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to persist analytics events")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Analytics events ingested", gin.H{
		"batchId":         req.BatchID,
		"eventsProcessed": len(req.Events),
	})
}
