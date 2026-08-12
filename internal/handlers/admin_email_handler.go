package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type AdminEmailHandler struct {
	svc       service.AdminEmailService
	schedules service.AdminEmailScheduleService
}

func NewAdminEmailHandler(svc service.AdminEmailService, schedules service.AdminEmailScheduleService) *AdminEmailHandler {
	return &AdminEmailHandler{svc: svc, schedules: schedules}
}

func (h *AdminEmailHandler) CreateSchedule(c *gin.Context) {
	var req models.UpsertAdminEmailScheduleRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.schedules.Create(&req, buildAdminEmailActor(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Email schedule created", item)
}
func (h *AdminEmailHandler) UpdateSchedule(c *gin.Context) {
	var req models.UpsertAdminEmailScheduleRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.schedules.Update(c.Param("id"), &req)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Email schedule not found")
			return
		}
		if strings.Contains(err.Error(), "currently being processed") || strings.Contains(err.Error(), "reload and retry") {
			utils.ErrorResponse(c, http.StatusConflict, err.Error())
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Email schedule updated", item)
}
func (h *AdminEmailHandler) GetSchedule(c *gin.Context) {
	item, err := h.schedules.Get(c.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Email schedule not found")
		} else {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load email schedule")
		}
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Email schedule loaded", item)
}
func (h *AdminEmailHandler) ListSchedules(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "20"), 1, 100)
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && status != "draft" && status != "active" && status != "paused" && status != "completed" && status != "failed" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid schedule status filter")
		return
	}
	items, total, err := h.schedules.List(page, limit, status)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Email schedule not found")
			return
		}
		if strings.Contains(err.Error(), "currently being processed") || strings.Contains(err.Error(), "reload and retry") {
			utils.ErrorResponse(c, http.StatusConflict, err.Error())
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load email schedules")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Email schedules loaded", gin.H{"data": items, "total": total, "page": page, "limit": limit, "totalPages": (total + int64(limit) - 1) / int64(limit)})
}
func (h *AdminEmailHandler) SetScheduleStatus(c *gin.Context) {
	var req struct {
		Status models.AdminEmailScheduleStatus `json:"status" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.schedules.SetStatus(c.Param("id"), req.Status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Email schedule status updated", item)
}
func (h *AdminEmailHandler) DeleteSchedule(c *gin.Context) {
	if err := h.schedules.Delete(c.Param("id")); err != nil {
		utils.ErrorResponse(c, http.StatusConflict, "Only inactive schedules can be deleted")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Email schedule deleted", gin.H{"id": c.Param("id")})
}
func (h *AdminEmailHandler) ListScheduleRuns(c *gin.Context) {
	items, err := h.schedules.ListRuns(c.Param("id"), parseIntClamp(c.DefaultQuery("limit", "20"), 1, 100))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load schedule runs")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Email schedule runs loaded", items)
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
