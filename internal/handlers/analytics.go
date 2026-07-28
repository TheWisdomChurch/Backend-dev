package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/database"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/metrics"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AnalyticsHandler struct {
	svc            service.AnalyticsService
	decisionEngine service.DecisionSupportService
}

var analyticsIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,119}$`)

func NewAnalyticsHandler(db *database.Database, redisCache *cache.RedisClient) *AnalyticsHandler {
	repo := repository.NewAnalyticsRepository(db)
	return &AnalyticsHandler{
		svc:            service.NewAnalyticsService(repo),
		decisionEngine: service.NewDecisionSupportService(db, redisCache),
	}
}

func (h *AnalyticsHandler) GetAdminAnalytics(c *gin.Context) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	result, err := h.svc.GetAdminAnalytics(ctx)
	if err != nil {
		applog.L().Error("admin analytics query failed", "error", err)
		metrics.RecordAnalyticsQuery("admin", "error", time.Since(started))
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}
	metrics.RecordAnalyticsQuery("admin", "success", time.Since(started))
	utils.OKMsg(c, "Analytics retrieved successfully", result)
}

func (h *AnalyticsHandler) GetDecisionInsights(c *gin.Context) {
	started := time.Now()
	if h.decisionEngine == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Decision insights engine is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	insights, err := h.decisionEngine.GetInsights(ctx)
	if err != nil {
		applog.L().Error("decision insights query failed", "error", err)
		metrics.RecordAnalyticsQuery("insights", "error", time.Since(started))
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute decision insights")
		return
	}
	metrics.RecordAnalyticsQuery("insights", "success", time.Since(started))
	utils.OKMsg(c, "Decision insights retrieved successfully", insights)
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
	req.BatchID = strings.TrimSpace(req.BatchID)
	req.Session.SessionID = strings.TrimSpace(req.Session.SessionID)
	if !analyticsIdentifierPattern.MatchString(req.BatchID) || !analyticsIdentifierPattern.MatchString(req.Session.SessionID) || len(req.Events) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "batchId, session.sessionId and events are required")
		metrics.RecordAnalyticsIngest("rejected", 0)
		return
	}
	if len(req.Events) > 100 {
		utils.ErrorResponse(c, http.StatusRequestEntityTooLarge, "at most 100 analytics events are allowed per batch")
		metrics.RecordAnalyticsIngest("rejected", 0)
		return
	}

	// Never trust a client-asserted user ID. Auth middleware is optional on this
	// public collection endpoint, so identity is attached only when verified.
	userID, _ := middleware.GetUserIDFromContext(c)
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	now := time.Now().UTC()
	normalized := make([]models.AnalyticsEvent, 0, len(req.Events))
	for _, item := range req.Events {
		category := strings.ToLower(strings.TrimSpace(item.Category))
		action := strings.ToLower(strings.TrimSpace(item.Action))
		if !analyticsIdentifierPattern.MatchString(category) || len(category) > 80 || !analyticsIdentifierPattern.MatchString(action) || len(action) > 80 {
			utils.ErrorResponse(c, http.StatusBadRequest, "analytics category and action must be valid identifiers")
			metrics.RecordAnalyticsIngest("rejected", 0)
			return
		}
		occurredAt := time.Unix(item.Timestamp, 0).UTC()
		if item.Timestamp > 1_000_000_000_000 {
			occurredAt = time.UnixMilli(item.Timestamp).UTC()
		}
		if occurredAt.Before(now.AddDate(0, 0, -7)) || occurredAt.After(now.Add(5*time.Minute)) {
			utils.ErrorResponse(c, http.StatusBadRequest, "analytics event timestamp is outside the accepted window")
			metrics.RecordAnalyticsIngest("rejected", 0)
			return
		}
		var clientEventID *string
		if id := strings.TrimSpace(item.ID); id != "" {
			if !analyticsIdentifierPattern.MatchString(id) {
				utils.ErrorResponse(c, http.StatusBadRequest, "analytics event id is invalid")
				metrics.RecordAnalyticsIngest("rejected", 0)
				return
			}
			clientEventID = &id
		}
		normalized = append(normalized, models.AnalyticsEvent{
			BatchID: req.BatchID, SessionID: req.Session.SessionID, UserID: userIDPtr,
			ClientEventID: clientEventID, Category: category, Action: action, OccurredAt: occurredAt,
		})
	}

	rawPayload, err := json.Marshal(gin.H{"batchId": req.BatchID, "sessionId": req.Session.SessionID, "events": normalized})
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

	if err := h.svc.IngestBatch(ctx, batch, normalized); err != nil {
		if errors.Is(err, repository.ErrDuplicateAnalyticsBatch) {
			metrics.RecordAnalyticsIngest("duplicate", 0)
			utils.OKMsg(c, "Analytics batch already ingested", gin.H{
				"batchId": req.BatchID, "eventsProcessed": 0, "duplicate": true,
			})
			return
		}
		metrics.RecordAnalyticsIngest("failed", 0)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to persist analytics events")
		return
	}
	metrics.RecordAnalyticsIngest("accepted", len(normalized))

	utils.OKMsg(c, "Analytics events ingested", gin.H{
		"batchId":         req.BatchID,
		"eventsProcessed": len(req.Events),
	})
}
