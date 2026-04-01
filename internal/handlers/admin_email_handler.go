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

func (h *AdminEmailHandler) GetMarketingSummary(c *gin.Context) {
	resp, err := h.svc.GetMarketingSummary()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load email marketing summary")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Email marketing summary loaded", resp)
}

func (h *AdminEmailHandler) ListAudienceForms(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "12"), 1, 100)

	items, total, err := h.svc.ListAudienceForms(page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load form audiences")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Form audiences loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *AdminEmailHandler) PreviewAudience(c *gin.Context) {
	limit := parseIntClamp(c.DefaultQuery("limit", "25"), 1, 200)
	formIDs := parseAdminEmailFormIDsQuery(c)

	resp, err := h.svc.PreviewAudience(formIDs, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Audience preview loaded", resp)
}

func parseAdminEmailFormIDsQuery(c *gin.Context) []string {
	if c == nil {
		return nil
	}

	rawValues := append([]string{}, c.QueryArray("formIds")...)
	if csv := strings.TrimSpace(c.Query("formIds")); csv != "" {
		rawValues = append(rawValues, strings.Split(csv, ",")...)
	}

	formIDs := make([]string, 0, len(rawValues))
	seen := make(map[string]struct{}, len(rawValues))
	for _, raw := range rawValues {
		formID := strings.TrimSpace(raw)
		if formID == "" {
			continue
		}
		if _, exists := seen[formID]; exists {
			continue
		}
		seen[formID] = struct{}{}
		formIDs = append(formIDs, formID)
	}
	return formIDs
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
