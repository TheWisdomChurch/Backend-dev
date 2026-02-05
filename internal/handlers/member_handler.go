package handlers

import (
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

type MemberHandler struct {
	svc service.MemberService
}

func NewMemberHandler(svc service.MemberService) *MemberHandler {
	return &MemberHandler{svc: svc}
}

func (h *MemberHandler) List(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "10"), 1, 100)

	var activePtr *bool
	if v := strings.TrimSpace(c.Query("active")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, "active must be true or false")
			return
		}
		activePtr = &b
	}

	items, total, err := h.svc.List(page, limit, activePtr)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load members")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Members loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *MemberHandler) Create(c *gin.Context) {
	var req models.CreateMemberRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	member, err := h.svc.Create(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Member created", member)
}

func (h *MemberHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req models.UpdateMemberRequest
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

func (h *MemberHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.Delete(id); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Member deleted", nil)
}

func (h *MemberHandler) BirthdayStats(c *gin.Context) {
	stats, err := h.svc.BirthdayStats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load birthday stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthday stats retrieved", stats)
}

func (h *MemberHandler) BirthdaysByMonth(c *gin.Context) {
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

func (h *MemberHandler) BirthdaysToday(c *gin.Context) {
	items, err := h.svc.BirthdaysToday(time.Now())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load today's birthdays")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Today's birthdays retrieved", gin.H{
		"data": items,
	})
}

func (h *MemberHandler) SendBirthdaysToday(c *gin.Context) {
	now := time.Now()
	result, err := h.svc.SendBirthdayGreetings(int(now.Month()), now.Day())
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Birthday emails queued/sent", result)
}
