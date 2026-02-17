package handlers

import (
	"html/template"
	"net/http"
	"strings"
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

func (h *FormHandler) ListAdminForms(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

	items, total, err := h.svc.List(page, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load forms")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Forms loaded", gin.H{
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
	utils.SuccessResponse(c, http.StatusOK, "Form loaded", form)
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
	utils.SuccessResponse(c, http.StatusCreated, "Form created", form)
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

	utils.SuccessResponse(c, http.StatusOK, "Form updated", form)
}

func (h *FormHandler) DeleteAdminForm(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.Delete(id); err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete form")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Form deleted", nil)
}

func (h *FormHandler) PublishAdminForm(c *gin.Context) {
	id := c.Param("id")
	slug, publicURL, err := h.svc.Publish(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	resp := gin.H{"slug": slug}
	if publicURL != nil {
		resp["publicUrl"] = *publicURL
	}
	utils.SuccessResponse(c, http.StatusOK, "Form published", resp)
}

func (h *FormHandler) ListAdminSubmissions(c *gin.Context) {
	formID := c.Param("id")
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

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

	utils.SuccessResponse(c, http.StatusOK, "Submissions loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *FormHandler) GetFormSubmissionStats(c *gin.Context) {
	formID := c.Param("id")
	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	stats, err := h.svc.StatsByForm(formID, start, end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load submission stats")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Submission stats retrieved", stats)
}

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

func (h *FormHandler) GetPublicForm(c *gin.Context) {
	slug := c.Param("slug")

	payload, err := h.svc.GetPublic(slug)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		if err == service.ErrFormExpired || err == service.ErrFormClosed {
			utils.ErrorResponse(c, http.StatusGone, err.Error())
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load form")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Form loaded", payload)
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
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "Form not found")
			return
		}
		if err == service.ErrFormExpired || err == service.ErrFormClosed {
			utils.ErrorResponse(c, http.StatusGone, err.Error())
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Registration submitted", nil)
}

func (h *FormHandler) ConfirmCalendarOptIn(c *gin.Context) {
	slug := c.Param("slug")
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "token is required")
		return
	}

	payload, err := h.svc.ConfirmCalendarOptIn(slug, token)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "calendar link not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	title := template.HTMLEscapeString(payload.EventTitle)
	dateValue := template.HTMLEscapeString(payload.EventDate)
	timeValue := template.HTMLEscapeString(payload.EventTime)
	locationValue := template.HTMLEscapeString(payload.EventLocation)

	googleLink := ""
	if strings.TrimSpace(payload.GoogleURL) != "" {
		googleLink = "<a href=\"" + template.HTMLEscapeString(payload.GoogleURL) + "\" style=\"display:inline-block;margin-right:10px;padding:10px 16px;background:#0f172a;color:#fff;text-decoration:none;border-radius:10px;font-weight:700;\">Open Google Calendar</a>"
	}
	icsLink := ""
	if strings.TrimSpace(payload.ICSURL) != "" {
		icsLink = "<a href=\"" + template.HTMLEscapeString(payload.ICSURL) + "\" style=\"display:inline-block;padding:10px 16px;background:#e2e8f0;color:#0f172a;text-decoration:none;border-radius:10px;font-weight:700;\">Download .ics</a>"
	}

	html := "<!DOCTYPE html><html><body style=\"font-family:Arial,sans-serif;padding:24px;background:#f8fafc;color:#0f172a;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:14px;padding:24px;\">" +
		"<h2 style=\"margin-top:0;\">Calendar reminder enabled</h2>" +
		"<p style=\"margin:0 0 12px;\">You will receive a gentle reminder email one day before this event.</p>" +
		"<p style=\"margin:0 0 6px;\"><strong>Event:</strong> " + title + "</p>" +
		"<p style=\"margin:0 0 6px;\"><strong>Date:</strong> " + dateValue + "</p>" +
		"<p style=\"margin:0 0 6px;\"><strong>Time:</strong> " + timeValue + "</p>" +
		"<p style=\"margin:0 0 16px;\"><strong>Location:</strong> " + locationValue + "</p>" +
		googleLink + icsLink +
		"</div></body></html>"

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func (h *FormHandler) DownloadCalendarICS(c *gin.Context) {
	slug := c.Param("slug")
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "token is required")
		return
	}

	filename, content, err := h.svc.BuildCalendarICS(slug, token)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "calendar link not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	c.Header("Content-Type", "text/calendar; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", content)
}

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
