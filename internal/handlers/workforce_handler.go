package handlers

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/email"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type WorkforceHandler struct {
	svc       service.WorkforceService
	notifySvc service.AdminNotificationService
	userRepo  repository.UserRepository
	sender    service.EmailSender
	tplRepo   repository.EmailTemplateRepository
	branding  email.Branding
}

func NewWorkforceHandler(
	svc service.WorkforceService,
	notifySvc service.AdminNotificationService,
	userRepo repository.UserRepository,
	sender service.EmailSender,
	tplRepo repository.EmailTemplateRepository,
	branding email.Branding,
) *WorkforceHandler {
	return &WorkforceHandler{
		svc:       svc,
		notifySvc: notifySvc,
		userRepo:  userRepo,
		sender:    sender,
		tplRepo:   tplRepo,
		branding:  branding,
	}
}

func (h *WorkforceHandler) currentUser(c *gin.Context) *models.User {
	if h.userRepo == nil {
		return nil
	}
	userID, ok := middleware.GetUserIDFromContext(c)
	if !ok {
		return nil
	}
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		return nil
	}
	return user
}

func (h *WorkforceHandler) List(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 10, 300)
	if !ok {
		return
	}

	department := c.Query("department")
	status, ok := parseEnumQuery(c, "status", "status", []string{"pending", "new", "serving", "not_serving"})
	if !ok {
		return
	}

	items, total, err := h.svc.List(page, limit, department, status)
	if err != nil {
		applog.L().Warn("workforce admin list failed", "page", page, "limit", limit, "department", department, "status", status, "error", err)
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
	h.sendWorkforceConfirmation(*member, "workforce_application_confirmation", "Pending Review")

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
	h.sendWorkforceConfirmation(*member, "workforce_serving_confirmation", "Serving")

	utils.SuccessResponse(c, http.StatusCreated, "Workforce profile submitted", member)
}

// LookupByEmail is a public, rate-limited endpoint used by the "existing member"
// self-service form to pre-fill known details. It intentionally returns only
// non-sensitive profile fields (name/phone/department) — never marital status,
// notes, or other pastoral-care-adjacent data — to limit exposure if the lookup
// is abused for email enumeration.
func (h *WorkforceHandler) LookupByEmail(c *gin.Context) {
	email := strings.TrimSpace(c.Query("email"))
	if email == "" || !strings.Contains(email, "@") || len(email) > 254 {
		utils.ErrorResponse(c, http.StatusBadRequest, "A valid email is required")
		return
	}

	member, err := h.svc.LookupByEmail(email)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "No profile found for this email")
			return
		}
		applog.L().Warn("workforce lookup failed", "email", email, "error", err)
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to look up profile")
		return
	}

	phone := ""
	if member.Phone != nil {
		phone = strings.TrimSpace(*member.Phone)
	}

	utils.SuccessResponse(c, http.StatusOK, "Profile found", gin.H{
		"firstName":  member.FirstName,
		"lastName":   member.LastName,
		"fullName":   strings.TrimSpace(member.FirstName + " " + member.LastName),
		"phone":      phone,
		"department": member.Department,
	})
}

func (h *WorkforceHandler) Update(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "workforce member id")
	if !ok {
		return
	}

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
	id, ok := parseUUIDParam(c, "id", "workforce member id")
	if !ok {
		return
	}

	member, err := h.svc.Approve(id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Workforce request approved", member)
}

func (h *WorkforceHandler) RejectRegistration(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "workforce member id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason" binding:"required"`
	}
	if !validation.BindJSON(c, &body) {
		return
	}

	member, err := h.svc.RejectRegistration(id, body.Reason, h.currentUser(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Workforce registration rejected", member)
}

func (h *WorkforceHandler) Delete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "workforce member id")
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason" binding:"required"`
	}
	if !validation.BindJSON(c, &body) {
		return
	}

	req, err := h.svc.RequestDelete(id, body.Reason, h.currentUser(c))
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusAccepted, "Workforce delete request sent for super admin approval", req)
}

func (h *WorkforceHandler) ApproveDelete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "workforce member id")
	if !ok {
		return
	}

	if err := h.svc.ApproveDelete(id, h.currentUser(c)); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Workforce profile deleted", nil)
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

func (h *WorkforceHandler) sendWorkforceConfirmation(member models.WorkforceMember, templateKey string, statusLabel string) {
	if h.sender == nil {
		return
	}

	addr := ""
	if member.Email != nil {
		addr = strings.TrimSpace(*member.Email)
	}
	if addr == "" {
		return
	}

	recipient := strings.TrimSpace(strings.Join([]string{member.FirstName, member.LastName}, " "))
	subject := "Workforce registration received"
	data := map[string]any{
		"Branding":      h.branding,
		"RecipientName": recipient,
		"ReferenceID":   member.ID,
		"Department":    member.Department,
		"StatusLabel":   statusLabel,
		"Member":        member,
	}

	body, subjectOverride := h.renderActiveTemplateByKey(templateKey, data)
	if subjectOverride != "" {
		subject = subjectOverride
	}
	if strings.TrimSpace(body) == "" {
		body = email.RenderWorkforceConfirmationEmail(email.WorkforceConfirmationTemplateData{
			Branding:      h.branding,
			RecipientName: recipient,
			ReferenceID:   member.ID,
			Department:    member.Department,
			StatusLabel:   statusLabel,
		})
	}

	if err := h.sender.SendHTML(addr, subject, body); err != nil {
		applog.L().Warn("failed to send workforce confirmation", "to", addr, "error", err)
	}
}

func (h *WorkforceHandler) renderActiveTemplateByKey(templateKey string, data any) (body string, subject string) {
	if h.tplRepo == nil {
		return "", ""
	}
	tpl, err := h.tplRepo.GetActiveByKey(strings.TrimSpace(templateKey))
	if err != nil || tpl == nil || strings.TrimSpace(tpl.HTMLBody) == "" {
		return "", ""
	}

	compiled, err := htmltemplate.New("workforce").Option("missingkey=zero").Parse(tpl.HTMLBody)
	if err != nil {
		applog.L().Warn("failed to parse email template", "template_key", templateKey, "error", err)
		return "", ""
	}

	var buf bytes.Buffer
	if err := compiled.Execute(&buf, data); err != nil {
		applog.L().Warn("failed to render email template", "template_key", templateKey, "error", err)
		return "", ""
	}

	var subjectOut string
	if tpl.Subject != nil {
		subjectOut = strings.TrimSpace(*tpl.Subject)
	}
	return buf.String(), subjectOut
}
