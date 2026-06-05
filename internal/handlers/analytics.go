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
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AnalyticsHandler struct {
	svc            service.AnalyticsService
	decisionEngine service.DecisionSupportService
}

func NewAnalyticsHandler(db *database.Database, redisCache *cache.RedisClient) *AnalyticsHandler {
	repo := repository.NewAnalyticsRepository(db)
	return &AnalyticsHandler{
		svc:            service.NewAnalyticsService(repo),
		decisionEngine: service.NewDecisionSupportService(db, redisCache),
	}
}

func (h *AnalyticsHandler) GetAdminAnalytics(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	result, err := h.svc.GetAdminAnalytics(ctx)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Analytics retrieved successfully", result)
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
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	rawPayload, err := json.Marshal(req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to serialize analytics payload")
		return
	}

	batch := &models.AnalyticsBatch{
		BatchID:    req.BatchID,
		SessionID:  req.Session.SessionID,
		UserID:     userIDPtr,
		EventCount: len(req.Events),
		Payload:    rawPayload,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	if err := h.svc.IngestBatch(ctx, batch); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to persist analytics events")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Analytics events ingested", gin.H{
		"batchId":         req.BatchID,
		"eventsProcessed": len(req.Events),
	})
}
