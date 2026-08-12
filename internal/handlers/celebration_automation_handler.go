package handlers

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type CelebrationAutomationHandler struct {
	svc service.CelebrationAutomationService
}

func NewCelebrationAutomationHandler(svc service.CelebrationAutomationService) *CelebrationAutomationHandler {
	return &CelebrationAutomationHandler{svc: svc}
}
func (h *CelebrationAutomationHandler) GetStatus(c *gin.Context) {
	result, err := h.svc.GetStatus(time.Now())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load celebration automation status")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Celebration automation status loaded", result)
}
func (h *CelebrationAutomationHandler) UpdateConfig(c *gin.Context) {
	var req models.UpdateCelebrationAutomationConfigRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	result, err := h.svc.UpdateConfig(&req, buildAdminEmailActor(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Celebration automation configuration updated", result)
}
func (h *CelebrationAutomationHandler) RunNow(c *gin.Context) {
	worker := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if worker == "" {
		worker = "celebration-admin"
	}
	result, err := h.svc.ProcessDue(c.Request.Context(), time.Now(), worker, "manual")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Celebration automation run processed", result)
}
func (h *CelebrationAutomationHandler) ListRuns(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "20"), 1, 100)
	items, total, err := h.svc.ListRuns(page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load celebration runs")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Celebration runs loaded", gin.H{"data": items, "total": total, "page": page, "limit": limit, "totalPages": (total + int64(limit) - 1) / int64(limit)})
}
func (h *CelebrationAutomationHandler) ListDeliveries(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "50"), 1, 200)
	items, total, err := h.svc.ListDeliveries(c.Param("id"), page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load celebration deliveries")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Celebration deliveries loaded", gin.H{"data": items, "total": total, "page": page, "limit": limit, "totalPages": (total + int64(limit) - 1) / int64(limit)})
}
