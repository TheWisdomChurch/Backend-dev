package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type AdminEmailHandler struct {
	svc service.AdminEmailService
}

func NewAdminEmailHandler(svc service.AdminEmailService) *AdminEmailHandler {
	return &AdminEmailHandler{svc: svc}
}

func (h *AdminEmailHandler) SendComposeEmail(c *gin.Context) {
	var req models.SendAdminComposeEmailRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	resp, err := h.svc.SendComposeEmail(&req, buildAdminEmailActor(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Compose email sent", resp)
}

func (h *AdminEmailHandler) ListComposeHistory(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

	items, total, err := h.svc.ListDeliveries(page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load email history")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Compose email history loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func buildAdminEmailActor(c *gin.Context) *models.AdminEmailSendActor {
	if c == nil {
		return nil
	}

	actor := &models.AdminEmailSendActor{}
	if userID, ok := middleware.GetUserIDFromContext(c); ok {
		actor.UserID = strings.TrimSpace(userID)
	}
	if raw, exists := c.Get("email"); exists {
		if email, ok := raw.(string); ok {
			actor.Email = strings.TrimSpace(email)
		}
	}
	if raw, exists := c.Get("role"); exists {
		if role, ok := raw.(string); ok {
			actor.Role = strings.TrimSpace(role)
		}
	}
	if actor.UserID == "" && actor.Email == "" && actor.Role == "" {
		return nil
	}
	return actor
}
