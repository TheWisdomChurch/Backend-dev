package handlers

import (
	"errors"
	"net/http"
	"strings"

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
	if token := strings.TrimSpace(c.Query("token")); token != "" {
		if err := h.svc.UnsubscribeByToken(token); err != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, "Invalid unsubscribe link")
			return
		}
		utils.SuccessResponse(c, http.StatusOK, "Unsubscribed successfully", nil)
		return
	}
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
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid unsubscribe link")
		return
	}

	if err := h.svc.UnsubscribeByToken(token); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Subscriber not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid unsubscribe link")
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
		`<html><body style="font-family:Arial,sans-serif;padding:24px;">
			<h3>You have been unsubscribed.</h3>
			<p>You will no longer receive updates from us.</p>
		</body></html>`))
}

func (h *NotificationHandler) SubscribeByLink(c *gin.Context) {
	emailAddr := c.Query("email")
	if emailAddr == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Email is required")
		return
	}

	name := strings.TrimSpace(c.Query("name"))
	source := strings.TrimSpace(c.Query("source"))
	req := &models.SubscribeRequest{
		Email: emailAddr,
	}
	if name != "" {
		req.Name = &name
	}
	if source != "" {
		req.Source = &source
	}

	if _, err := h.svc.Subscribe(req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
		`<html><body style="font-family:Arial,sans-serif;padding:24px;">
			<h3>Subscription confirmed.</h3>
			<p>You will now receive updates from us.</p>
		</body></html>`))
}

func (h *NotificationHandler) ListSubscribers(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 10, 100)
	if !ok {
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))
	source := strings.TrimSpace(c.Query("source"))

	items, total, err := h.svc.ListSubscribers(page, limit, status, search, source)
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

func (h *NotificationHandler) GetSubscriberSummary(c *gin.Context) {
	summary, err := h.svc.GetSubscriberSummary()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load subscriber summary")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Subscriber summary loaded", summary)
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
