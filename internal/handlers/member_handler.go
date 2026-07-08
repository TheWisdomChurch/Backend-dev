package handlers

import (
	"encoding/csv"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	applog "wisdomHouse-backend/internal/logger"
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
	page, limit, ok := parsePaginationQuery(c, 10, 300)
	if !ok {
		return
	}

	activePtr, ok := parseOptionalBoolQuery(c, "active", "active")
	if !ok {
		return
	}

	items, total, err := h.svc.List(page, limit, activePtr)
	if err != nil {
		log.Printf("member admin list failed page=%d limit=%d active=%v: %v", page, limit, activePtr, err)
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

func (h *MemberHandler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load member stats")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Member stats retrieved", stats)
}

func (h *MemberHandler) NewMemberDashboard(c *gin.Context) {
	dashboard, err := h.svc.NewMemberDashboard(time.Now())
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load new member dashboard")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "New member dashboard retrieved", dashboard)
}

func (h *MemberHandler) ListNewMemberSubmissions(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 25, 500)
	if !ok {
		return
	}
	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	items, total, err := h.svc.ListNewMemberSubmissions(page, limit, start, end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load new member submissions")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "New member submissions loaded", gin.H{
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
	id, ok := parseUUIDParam(c, "id", "member id")
	if !ok {
		return
	}

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
	id, ok := parseUUIDParam(c, "id", "member id")
	if !ok {
		return
	}
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
	month, ok := parseMonthPathParam(c, "month")
	if !ok {
		return
	}
	items, err := h.svc.BirthdaysByMonth(month)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "month must be 1-12") {
			utils.ErrorResponse(c, http.StatusBadRequest, "month must be 1-12")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load birthdays")
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

func (h *MemberHandler) SendAnnouncement(c *gin.Context) {
	var req models.SendMemberEmailRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	resp, err := h.svc.SendAnnouncement(&req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Member announcement sent", resp)
}

// ImportCSV accepts a multipart CSV upload and bulk-imports members.
// Required columns (case-insensitive): first_name, last_name, email
// Optional columns: phone, birthday, is_active
//
// POST /api/v1/admin/members/import
func (h *MemberHandler) ImportCSV(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // variable columns OK

	headers, err := reader.Read()
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "CSV is empty or unreadable")
		return
	}

	// Normalise header names → column index map
	colIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	required := []string{"first_name", "last_name", "email"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			utils.ErrorResponse(c, http.StatusBadRequest, "CSV missing required column: "+col)
			return
		}
	}

	col := func(row []string, name string) string {
		idx, ok := colIdx[name]
		if !ok || idx >= len(row) {
			return ""
		}
		return row[idx]
	}

	records, err := reader.ReadAll()
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Failed to parse CSV: "+err.Error())
		return
	}
	if len(records) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "CSV has no data rows")
		return
	}

	rows := make([]service.CSVMemberRow, 0, len(records))
	for _, r := range records {
		rows = append(rows, service.CSVMemberRow{
			FirstName: col(r, "first_name"),
			LastName:  col(r, "last_name"),
			Email:     col(r, "email"),
			Phone:     col(r, "phone"),
			Birthday:  col(r, "birthday"),
			IsActive:  col(r, "is_active"),
		})
	}

	result, err := h.svc.BulkImport(rows)
	if err != nil {
		log.Printf("member CSV import failed: %v", err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Import failed")
		return
	}

	status := http.StatusOK
	if result.Imported == 0 {
		status = http.StatusUnprocessableEntity
	}
	utils.SuccessResponse(c, status, "Import complete", result)
}
