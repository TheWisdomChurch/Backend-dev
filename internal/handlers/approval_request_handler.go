package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type ApprovalRequestHandler struct {
	svc  service.ApprovalService
	repo *repository.ApprovalRequestRepository
}

func NewApprovalRequestHandler(svc service.ApprovalService, repo *repository.ApprovalRequestRepository) *ApprovalRequestHandler {
	return &ApprovalRequestHandler{svc: svc, repo: repo}
}

func (h *ApprovalRequestHandler) List(c *gin.Context) {
	typeFilter := strings.TrimSpace(c.Query("type"))
	statusFilter := strings.TrimSpace(c.Query("status"))
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}

	var types []models.ApprovalRequestType
	if typeFilter != "" {
		for _, t := range strings.Split(typeFilter, ",") {
			tt := strings.TrimSpace(t)
			if tt != "" {
				types = append(types, models.ApprovalRequestType(tt))
			}
		}
	}

	var statuses []models.ApprovalRequestStatus
	if statusFilter != "" {
		for _, s := range strings.Split(statusFilter, ",") {
			ss := strings.TrimSpace(s)
			if ss != "" {
				statuses = append(statuses, models.ApprovalRequestStatus(ss))
			}
		}
	}

	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.svc.ListRequests(types, statuses, start, end, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load requests")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Requests loaded", items)
}

func (h *ApprovalRequestHandler) Timeline(c *gin.Context) {
	days := 7
	if raw := strings.TrimSpace(c.Query("days")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 1 && v <= 90 {
			days = v
		}
	}
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -days+1)

	created, err := h.repo.CountCreatedByDay(&start, &end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load timeline")
		return
	}
	approved, err := h.repo.CountApprovedByDay(&start, &end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load timeline")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Timeline loaded", gin.H{
		"start":    start,
		"end":      end,
		"created":  created,
		"approved": approved,
	})
}
