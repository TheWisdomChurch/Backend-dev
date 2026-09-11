package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type AttendanceHandler struct {
	svc service.AttendanceService
}

func NewAttendanceHandler(svc service.AttendanceService) *AttendanceHandler {
	return &AttendanceHandler{svc: svc}
}

func (h *AttendanceHandler) ListServiceTypes(c *gin.Context) {
	var campusID *string
	if v := c.Query("campus_id"); v != "" {
		campusID = &v
	}
	rows, err := h.svc.ListServiceTypes(c.Request.Context(), campusID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load service types")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Service types retrieved", rows)
}

func (h *AttendanceHandler) CreateServiceType(c *gin.Context) {
	var st models.ServiceType
	if err := c.ShouldBindJSON(&st); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if err := h.svc.CreateServiceType(c.Request.Context(), &st); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Service type created", st)
}

func (h *AttendanceHandler) CreateSession(c *gin.Context) {
	var req service.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	var createdByID *string
	if v, ok := c.Get("user_id"); ok {
		if id, ok := v.(string); ok && id != "" {
			createdByID = &id
		}
	}
	session, err := h.svc.CreateSession(c.Request.Context(), req, createdByID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Attendance session created", session)
}

func (h *AttendanceHandler) UpdateSession(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if err := h.svc.UpdateSession(c.Request.Context(), id, body); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Attendance session not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "session updated", nil)
}

func (h *AttendanceHandler) GetSession(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	session, err := h.svc.GetSession(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.ErrorResponse(c, http.StatusNotFound, "Attendance session not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load attendance session")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Attendance session retrieved", session)
}

func (h *AttendanceHandler) ListSessions(c *gin.Context) {
	var campusID *string
	if v := c.Query("campus_id"); v != "" {
		campusID = &v
	}
	var from, to *time.Time
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = &t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = &t
		}
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	sessions, total, err := h.svc.ListSessions(c.Request.Context(), campusID, from, to, limit, (page-1)*limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load attendance sessions")
		return
	}
	utils.PaginatedSuccessResponse(c, http.StatusOK, sessions, page, limit, int(total))
}

func (h *AttendanceHandler) CheckIn(c *gin.Context) {
	var req service.CheckInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	rec, err := h.svc.CheckIn(c.Request.Context(), req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Checked in", rec)
}

func (h *AttendanceHandler) ListRecords(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	records, err := h.svc.ListRecords(c.Request.Context(), sessionID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load attendance records")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Attendance records retrieved", records)
}

func (h *AttendanceHandler) MemberHistory(c *gin.Context) {
	memberID := strings.TrimSpace(c.Param("member_id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	records, err := h.svc.MemberHistory(c.Request.Context(), memberID, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load member attendance history")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Member attendance history retrieved", records)
}
