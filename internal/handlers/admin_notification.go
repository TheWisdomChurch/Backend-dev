package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AdminNotificationHandler struct {
	svc service.AdminNotificationService
}

func NewAdminNotificationHandler(svc service.AdminNotificationService) *AdminNotificationHandler {
	return &AdminNotificationHandler{svc: svc}
}

func (h *AdminNotificationHandler) List(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}

	items, unread, err := h.svc.ListForUser(userID, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load notifications")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Notifications loaded", gin.H{
		"items":  items,
		"unread": unread,
	})
}

func (h *AdminNotificationHandler) MarkRead(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Missing notification id")
		return
	}
	if err := h.svc.MarkRead(userID, id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update notification")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Notification marked read", nil)
}

func (h *AdminNotificationHandler) MarkAllRead(c *gin.Context) {
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.svc.MarkAllRead(userID); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update notifications")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Notifications marked read", nil)
}
