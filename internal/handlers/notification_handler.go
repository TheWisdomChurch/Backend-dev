package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type NotificationHandler struct {
	svc service.NotificationService
}

func NewNotificationHandler(svc service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

func (h *NotificationHandler) Subscribe(c *gin.Context) {
	var req models.SubscribeRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	subscriber, err := h.svc.Subscribe(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Subscription saved", subscriber)
}

func (h *NotificationHandler) Unsubscribe(c *gin.Context) {
	var req models.UnsubscribeRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.svc.Unsubscribe(req.Email); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Subscriber not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Unsubscribed successfully", nil)
}

func (h *NotificationHandler) UnsubscribeByLink(c *gin.Context) {
	emailAddr := c.Query("email")
	if emailAddr == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Email is required")
		return
	}

	if err := h.svc.Unsubscribe(emailAddr); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Subscriber not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
		`<html><body style="font-family:Arial,sans-serif;padding:24px;">
			<h3>You have been unsubscribed.</h3>
			<p>You will no longer receive updates from us.</p>
		</body></html>`))
}

func (h *NotificationHandler) ListSubscribers(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

	items, total, err := h.svc.ListSubscribers(page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load subscribers")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Subscribers loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *NotificationHandler) SendNotification(c *gin.Context) {
	var req models.SendNotificationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	result, err := h.svc.SendNotification(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Notification queued/sent", result)
}
