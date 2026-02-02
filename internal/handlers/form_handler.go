// internal/handlers/form_handler.go
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type FormHandler struct {
	svc service.FormService
}

func NewFormHandler(svc service.FormService) *FormHandler {
	return &FormHandler{svc: svc}
}

/* =========================
   ADMIN
========================= */

func (h *FormHandler) ListAdminForms(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	items, total, err := h.svc.List(page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load forms")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *FormHandler) GetAdminForm(c *gin.Context) {
	id := c.Param("id")
	form, err := h.svc.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load form")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": form})
}

func (h *FormHandler) CreateAdminForm(c *gin.Context) {
	var req models.CreateFormRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	form, err := h.svc.Create(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": form})
}

func (h *FormHandler) UpdateAdminForm(c *gin.Context) {
	id := c.Param("id")

	var req models.UpdateFormRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	form, err := h.svc.Update(id, &req)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": form})
}

func (h *FormHandler) DeleteAdminForm(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.Delete(id); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete form")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Form deleted"})
}

func (h *FormHandler) PublishAdminForm(c *gin.Context) {
	id := c.Param("id")
	slug, err := h.svc.Publish(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"slug": slug}})
}

// ListAdminSubmissions returns submissions for a specific form with pagination and optional date range.
func (h *FormHandler) ListAdminSubmissions(c *gin.Context) {
	formID := c.Param("id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	items, total, err := h.svc.ListSubmissions(formID, page, limit, start, end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load submissions")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

// GetFormStats returns total counts, per-form counts, and recent submissions for analytics.
func (h *FormHandler) GetFormStats(c *gin.Context) {
	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	stats, err := h.svc.Stats(start, end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load form stats")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Form stats retrieved", stats)
}

/* =========================
   PUBLIC
========================= */

func (h *FormHandler) GetPublicForm(c *gin.Context) {
	slug := c.Param("slug")

	payload, err := h.svc.GetPublic(slug)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load form")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": payload})
}

func (h *FormHandler) SubmitPublicForm(c *gin.Context) {
	slug := c.Param("slug")

	var req models.SubmitFormRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if req.Values == nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Values are required")
		return
	}

	err := h.svc.Submit(slug, &req)
	if err != nil {
		// Service returns readable error strings; map to 400 except not found
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Registration submitted"})
}

// parseTimeRange parses optional start/end query params as RFC3339 timestamps.
func parseTimeRange(c *gin.Context) (*time.Time, *time.Time, error) {
	startStr := c.Query("start")
	endStr := c.Query("end")

	var start *time.Time
	var end *time.Time

	if startStr != "" {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, nil, err
		}
		start = &t
	}
	if endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, nil, err
		}
		end = &t
	}

	return start, end, nil
}
