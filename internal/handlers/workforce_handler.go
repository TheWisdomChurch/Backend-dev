package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type WorkforceHandler struct {
	svc       service.WorkforceService
	notifySvc service.AdminNotificationService
}

func NewWorkforceHandler(svc service.WorkforceService, notifySvc service.AdminNotificationService) *WorkforceHandler {
	return &WorkforceHandler{svc: svc, notifySvc: notifySvc}
}

func (h *WorkforceHandler) List(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

	department := c.Query("department")
	status := c.Query("status")

	items, total, err := h.svc.List(page, limit, department, status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load workforce")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Workforce loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *WorkforceHandler) Create(c *gin.Context) {
	var req models.CreateWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.SourceChannel) == "" {
		req.SourceChannel = "admin:web:workforce"
	}

	member, err := h.svc.Create(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Member created", member)
}

func (h *WorkforceHandler) Apply(c *gin.Context) {
	var req models.CreateWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.SourceChannel) == "" {
		req.SourceChannel = "frontend:web:workforce:new"
	}

	member, err := h.svc.CreateApplication(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if h.notifySvc != nil {
		fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
		title := "New workforce application"
		message := fmt.Sprintf("%s applied for the workforce (%s).", fullName, member.Department)
		entityType := "workforce"
		entityID := member.ID
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "workforce_application",
			Title:      title,
			Message:    message,
			EntityType: &entityType,
			EntityID:   &entityID,
			Roles:      []string{"admin", "super_admin"},
		})
	}

	utils.SuccessResponse(c, http.StatusCreated, "Application submitted", member)
}

func (h *WorkforceHandler) ApplyServing(c *gin.Context) {
	var req models.CreateWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.SourceChannel) == "" {
		req.SourceChannel = "frontend:web:workforce:serving"
	}

	member, err := h.svc.RegisterExisting(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if h.notifySvc != nil {
		fullName := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
		title := "Existing worker profile update"
		message := fmt.Sprintf("%s submitted workforce profile details (%s).", fullName, member.Department)
		entityType := "workforce"
		entityID := member.ID
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "workforce_profile_update",
			Title:      title,
			Message:    message,
			EntityType: &entityType,
			EntityID:   &entityID,
			Roles:      []string{"admin", "super_admin"},
		})
	}

	utils.SuccessResponse(c, http.StatusCreated, "Workforce profile submitted", member)
}

func (h *WorkforceHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req models.UpdateWorkforceRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Update(id, &req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Member updated", member)
}

func (h *WorkforceHandler) Approve(c *gin.Context) {
	id := c.Param("id")

	member, err := h.svc.Approve(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Workforce request approved", member)
}

func (h *WorkforceHandler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load workforce stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Workforce stats retrieved", stats)
}

func (h *WorkforceHandler) BirthdayStats(c *gin.Context) {
	stats, err := h.svc.BirthdayStats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load birthday stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthday stats retrieved", stats)
}

func (h *WorkforceHandler) BirthdaysByMonth(c *gin.Context) {
	raw := strings.TrimSpace(c.Param("month"))
	month, err := strconv.Atoi(raw)
	if err != nil || month < 1 || month > 12 {
		utils.ErrorResponse(c, http.StatusBadRequest, "month must be 1-12")
		return
	}
	items, err := h.svc.BirthdaysByMonth(month)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthdays retrieved", gin.H{
		"month": month,
		"data":  items,
	})
}

func (h *WorkforceHandler) BirthdaysToday(c *gin.Context) {
	items, err := h.svc.BirthdaysToday(time.Now())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load today's birthdays")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Today's birthdays retrieved", gin.H{
		"data": items,
	})
}

func (h *WorkforceHandler) SendBirthdaysToday(c *gin.Context) {
	now := time.Now()
	result, err := h.svc.SendBirthdayGreetings(int(now.Month()), now.Day())
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthday emails queued/sent", result)
}
