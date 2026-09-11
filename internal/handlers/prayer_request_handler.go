package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type PrayerRequestHandler struct {
	svc service.PrayerRequestService
}

func NewPrayerRequestHandler(svc service.PrayerRequestService) *PrayerRequestHandler {
	return &PrayerRequestHandler{svc: svc}
}

// Submit accepts a public prayer request (no auth required).
func (h *PrayerRequestHandler) Submit(c *gin.Context) {
	var req service.SubmitPrayerRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	pr, err := h.svc.Submit(c.Request.Context(), req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	// Never echo back the decrypted body on public submission — return only the ID.
	utils.SuccessResponse(c, http.StatusCreated, "Prayer request submitted", gin.H{"id": pr.ID, "status": pr.Status})
}

// Get returns a single prayer request with decrypted content (admin only).
func (h *PrayerRequestHandler) Get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	pr, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Prayer request not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load prayer request")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Prayer request retrieved", pr)
}

// List returns paginated prayer requests (admin only).
func (h *PrayerRequestHandler) List(c *gin.Context) {
	status := c.Query("status")
	category := c.Query("category")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	rows, total, err := h.svc.List(c.Request.Context(), status, category, limit, (page-1)*limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load prayer requests")
		return
	}
	utils.PaginatedSuccessResponse(c, http.StatusOK, rows, page, limit, int(total))
}

// UpdateStatus changes the prayer request status (admin only).
func (h *PrayerRequestHandler) UpdateStatus(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if !validation.BindJSON(c, &body) {
		return
	}
	if err := h.svc.UpdateStatus(c.Request.Context(), id, body.Status); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "status updated", nil)
}

// Assign assigns a prayer request to a staff member (admin only).
func (h *PrayerRequestHandler) Assign(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var body struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if !validation.BindJSON(c, &body) {
		return
	}
	if err := h.svc.AssignTo(c.Request.Context(), id, body.UserID); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "request assigned", nil)
}

// AddNotes adds encrypted pastoral notes (admin only).
func (h *PrayerRequestHandler) AddNotes(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var body struct {
		Notes string `json:"notes" binding:"required"`
	}
	if !validation.BindJSON(c, &body) {
		return
	}
	if err := h.svc.AddNotes(c.Request.Context(), id, body.Notes); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "notes saved", nil)
}

// Delete soft-deletes a prayer request (admin only).
func (h *PrayerRequestHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete prayer request")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "request deleted", nil)
}
