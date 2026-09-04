package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type WeddingAnniversaryHandler struct {
	svc service.WeddingAnniversaryService
}

func NewWeddingAnniversaryHandler(svc service.WeddingAnniversaryService) *WeddingAnniversaryHandler {
	return &WeddingAnniversaryHandler{svc: svc}
}

// upsertWeddingAnniversaryRequest is the admin create/update payload.
type upsertWeddingAnniversaryRequest struct {
	SubjectType string `json:"subjectType" binding:"required"`
	SubjectID   string `json:"subjectId" binding:"required"`
	models.WeddingAnniversaryInput
}

type archiveWeddingAnniversaryRequest struct {
	Reason string `json:"reason"`
}

func (h *WeddingAnniversaryHandler) List(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 50, 200)
	if !ok {
		return
	}
	f := repository.WeddingAnniversaryFilter{
		Status:      strings.TrimSpace(c.Query("status")),
		SubjectType: strings.TrimSpace(c.Query("subjectType")),
		NeedsSpouse: strings.EqualFold(strings.TrimSpace(c.Query("needsSpouse")), "true"),
		Offset:      (page - 1) * limit,
		Limit:       limit,
	}
	if m := parseIntClamp(c.Query("month"), 0, 12); m >= 1 {
		f.Month = m
	}

	rows, total, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		applog.L().Warn("wedding anniversary list failed", "error", err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load wedding anniversaries")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversaries retrieved", gin.H{
		"data": rows, "total": total, "page": page, "limit": limit,
	})
}

func (h *WeddingAnniversaryHandler) Get(c *gin.Context) {
	row, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Wedding anniversary not found")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversary retrieved", row)
}

func (h *WeddingAnniversaryHandler) Upsert(c *gin.Context) {
	var req upsertWeddingAnniversaryRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if !models.ValidWeddingAnniversarySubjectType(strings.TrimSpace(req.SubjectType)) {
		utils.ErrorResponse(c, http.StatusBadRequest, "subjectType must be member, leadership, or workforce")
		return
	}
	row, err := h.svc.UpsertForSubject(
		c.Request.Context(),
		strings.TrimSpace(req.SubjectType),
		strings.TrimSpace(req.SubjectID),
		req.WeddingAnniversaryInput,
		models.WeddingAnniversarySourceAdmin,
		nil,
	)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversary saved", row)
}

func (h *WeddingAnniversaryHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete wedding anniversary")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversary deleted", nil)
}

func (h *WeddingAnniversaryHandler) Archive(c *gin.Context) {
	var req archiveWeddingAnniversaryRequest
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.Archive(c.Request.Context(), c.Param("id"), req.Reason); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversary archived", nil)
}

func (h *WeddingAnniversaryHandler) Unarchive(c *gin.Context) {
	if err := h.svc.Unarchive(c.Request.Context(), c.Param("id")); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversary reactivated", nil)
}

func (h *WeddingAnniversaryHandler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load wedding anniversary stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversary stats retrieved", stats)
}

func (h *WeddingAnniversaryHandler) ByMonth(c *gin.Context) {
	month, ok := parseMonthPathParam(c, "month")
	if !ok {
		return
	}
	rows, _, err := h.svc.List(c.Request.Context(), repository.WeddingAnniversaryFilter{Month: month, Status: "active", Limit: 200})
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load wedding anniversaries")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversaries retrieved", gin.H{"month": month, "data": rows})
}

func (h *WeddingAnniversaryHandler) Today(c *gin.Context) {
	now := time.Now()
	rows, err := h.svc.DueOn(c.Request.Context(), int(now.Month()), now.Day())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load today's wedding anniversaries")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Today's wedding anniversaries retrieved", gin.H{"data": rows})
}

func (h *WeddingAnniversaryHandler) SendToday(c *gin.Context) {
	now := time.Now()
	result, err := h.svc.SendGreetingsForDay(c.Request.Context(), int(now.Month()), now.Day())
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Wedding anniversary emails queued/sent", result)
}
