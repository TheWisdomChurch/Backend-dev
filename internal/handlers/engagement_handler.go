package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/email"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/sanitize"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type EngagementHandler struct {
	repo      repository.EngagementRepository
	notifySvc service.AdminNotificationService
	sender    service.EmailSender
	tplRepo   repository.EmailTemplateRepository
	branding  email.Branding
}

func NewEngagementHandler(
	db *database.Database,
	notifySvc service.AdminNotificationService,
	sender service.EmailSender,
	tplRepo repository.EmailTemplateRepository,
	branding email.Branding,
) *EngagementHandler {
	return &EngagementHandler{
		repo:      repository.NewEngagementRepository(db),
		notifySvc: notifySvc,
		sender:    sender,
		tplRepo:   tplRepo,
		branding:  branding,
	}
}

type CreatePastoralCareRequest struct {
	Title         string `json:"title" binding:"required"`
	FirstName     string `json:"firstName" binding:"required"`
	LastName      string `json:"lastName" binding:"required"`
	Phone         string `json:"phone" binding:"required"`
	Email         string `json:"email" binding:"required,email"`
	Address       string `json:"address" binding:"required"`
	EventDate     string `json:"eventDate" binding:"required"`
	EventType     string `json:"eventType" binding:"required"`
	ChurchRole    string `json:"churchRole" binding:"required"`
	CustomRole    string `json:"customRole"`
	Comments      string `json:"comments"`
	SourceChannel string `json:"sourceChannel"`
}

type CreateGivingIntentRequest struct {
	Title         string                 `json:"title" binding:"required"`
	Description   string                 `json:"description"`
	SourceChannel string                 `json:"sourceChannel"`
	Metadata      map[string]interface{} `json:"metadata"`
}

type CreateContactMessageRequest struct {
	FirstName     string                 `json:"firstName" binding:"required"`
	LastName      string                 `json:"lastName" binding:"required"`
	Email         string                 `json:"email" binding:"required,email"`
	Phone         string                 `json:"phone"`
	Topic         string                 `json:"topic"`
	Message       string                 `json:"message" binding:"required"`
	SourceChannel string                 `json:"sourceChannel"`
	Metadata      map[string]interface{} `json:"metadata"`
}

func (h *EngagementHandler) CreatePastoralCareRequest(c *gin.Context) {
	var req CreatePastoralCareRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	source := strings.TrimSpace(req.SourceChannel)
	if source == "" {
		source = "frontend:web:pastoral-care"
	}

	row := models.PastoralCareRequest{
		Title:         sanitize.Text(strings.TrimSpace(req.Title)),
		FirstName:     sanitize.Text(strings.TrimSpace(req.FirstName)),
		LastName:      sanitize.Text(strings.TrimSpace(req.LastName)),
		Phone:         strings.TrimSpace(req.Phone),
		Email:         strings.ToLower(strings.TrimSpace(req.Email)),
		Address:       sanitize.Text(strings.TrimSpace(req.Address)),
		EventDate:     strings.TrimSpace(req.EventDate),
		EventType:     sanitize.Text(strings.TrimSpace(req.EventType)),
		ChurchRole:    sanitize.Text(strings.TrimSpace(req.ChurchRole)),
		CustomRole:    sanitize.Text(strings.TrimSpace(req.CustomRole)),
		Comments:      sanitize.Text(strings.TrimSpace(req.Comments)),
		SourceChannel: source,
	}

	ctx, cancel := engagementContextWithTimeout()
	defer cancel()
	if err := h.repo.CreatePastoralCareRequest(ctx, &row); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to submit pastoral care request")
		return
	}

	if h.notifySvc != nil {
		fullName := strings.TrimSpace(strings.Join([]string{row.FirstName, row.LastName}, " "))
		title := "New pastoral care request"
		message := fmt.Sprintf("%s requested pastoral care (%s).", fullName, row.EventType)
		entityType := "pastoral_care_request"
		entityID := row.ID
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "pastoral_care_request",
			Title:      title,
			Message:    message,
			EntityType: &entityType,
			EntityID:   &entityID,
			Roles:      []string{"admin", "super_admin"},
		})
	}
	h.sendPastoralConfirmation(row)

	utils.SuccessResponse(c, http.StatusCreated, "Pastoral care request submitted", row)
}

func (h *EngagementHandler) CreateGivingIntent(c *gin.Context) {
	var req CreateGivingIntentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	source := strings.TrimSpace(req.SourceChannel)
	if source == "" {
		source = "frontend:web:online-giving"
	}

	row := models.GivingIntent{
		Title:         sanitize.Text(strings.TrimSpace(req.Title)),
		Description:   sanitize.Text(strings.TrimSpace(req.Description)),
		SourceChannel: source,
	}
	if len(req.Metadata) > 0 {
		raw, err := json.Marshal(req.Metadata)
		if err != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, "Invalid metadata format")
			return
		}
		row.Metadata = raw
	}

	ctx, cancel := engagementContextWithTimeout()
	defer cancel()
	if err := h.repo.CreateGivingIntent(ctx, &row); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to capture giving intent")
		return
	}

	if h.notifySvc != nil {
		title := "New giving intent"
		message := fmt.Sprintf("A giving intent was submitted for %s.", row.Title)
		entityType := "giving_intent"
		entityID := row.ID
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "giving_intent",
			Title:      title,
			Message:    message,
			EntityType: &entityType,
			EntityID:   &entityID,
			Roles:      []string{"admin", "super_admin"},
		})
	}
	h.sendGivingConfirmation(row)

	utils.SuccessResponse(c, http.StatusCreated, "Giving intent captured", row)
}

func (h *EngagementHandler) CreateContactMessage(c *gin.Context) {
	var req CreateContactMessageRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	source := strings.TrimSpace(req.SourceChannel)
	if source == "" {
		source = "frontend:web:contact"
	}

	row := models.ContactMessage{
		FirstName:     sanitize.Text(strings.TrimSpace(req.FirstName)),
		LastName:      sanitize.Text(strings.TrimSpace(req.LastName)),
		Email:         strings.ToLower(strings.TrimSpace(req.Email)),
		Phone:         strings.TrimSpace(req.Phone),
		Topic:         sanitize.Text(strings.TrimSpace(req.Topic)),
		Message:       sanitize.Text(strings.TrimSpace(req.Message)),
		SourceChannel: source,
		Metadata:      req.Metadata,
	}

	ctx, cancel := engagementContextWithTimeout()
	defer cancel()
	if err := h.repo.CreateContactMessage(ctx, &row); err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to submit contact message")
		return
	}

	if h.notifySvc != nil {
		fullName := strings.TrimSpace(strings.Join([]string{row.FirstName, row.LastName}, " "))
		title := "New contact message"
		message := fmt.Sprintf("%s sent a contact message (%s).", fullName, row.Topic)
		entityType := "contact_message"
		entityID := row.ID
		_ = h.notifySvc.NotifyRoles(service.AdminNotificationInput{
			Type:       "contact_message",
			Title:      title,
			Message:    message,
			EntityType: &entityType,
			EntityID:   &entityID,
			Roles:      []string{"admin", "super_admin"},
		})
	}

	utils.SuccessResponse(c, http.StatusCreated, "Contact message submitted", row)
}

func (h *EngagementHandler) ListPastoralCareRequests(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "20"), 1, 100)
	offset := (page - 1) * limit

	ctx, cancel := engagementContextWithTimeout()
	defer cancel()

	items, total, err := h.repo.ListPastoralCareRequests(ctx, offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load pastoral care requests")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Pastoral care requests loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *EngagementHandler) ListGivingIntents(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "20"), 1, 100)
	offset := (page - 1) * limit

	ctx, cancel := engagementContextWithTimeout()
	defer cancel()

	items, total, err := h.repo.ListGivingIntents(ctx, offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load giving intents")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Giving intents loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *EngagementHandler) ListContactMessages(c *gin.Context) {
	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "20"), 1, 100)
	offset := (page - 1) * limit

	ctx, cancel := engagementContextWithTimeout()
	defer cancel()

	items, total, err := h.repo.ListContactMessages(ctx, offset, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load contact messages")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Contact messages loaded", gin.H{
		"data":       items,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func engagementContextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (h *EngagementHandler) sendPastoralConfirmation(row models.PastoralCareRequest) {
	if h.sender == nil {
		return
	}

	addr := strings.TrimSpace(row.Email)
	if addr == "" {
		return
	}

	recipient := strings.TrimSpace(strings.Join([]string{row.FirstName, row.LastName}, " "))
	subject := "Pastoral care request received"
	data := map[string]any{
		"Branding":      h.branding,
		"RecipientName": recipient,
		"ReferenceID":   row.ID,
		"EventType":     row.EventType,
		"EventDate":     row.EventDate,
		"Request":       row,
	}

	body, subjectOverride := h.renderActiveTemplateByKey("pastoral_care_request_confirmation", data)
	if subjectOverride != "" {
		subject = subjectOverride
	}
	if strings.TrimSpace(body) == "" {
		body = email.RenderPastoralCareConfirmationEmail(email.PastoralCareConfirmationTemplateData{
			Branding:      h.branding,
			RecipientName: recipient,
			ReferenceID:   row.ID,
			EventType:     row.EventType,
			EventDate:     row.EventDate,
		})
	}

	if err := h.sender.SendHTML(addr, subject, body); err != nil {
		applog.L().Warn("failed to send pastoral care confirmation", "to", addr, "error", err)
	}
}

func (h *EngagementHandler) sendGivingConfirmation(row models.GivingIntent) {
	if h.sender == nil {
		return
	}

	// Giving intent endpoint currently does not require recipient email. If metadata.email exists, use it.
	addr := extractMetadataEmail(row.Metadata)
	if addr == "" {
		return
	}

	subject := "Giving request received"
	data := map[string]any{
		"Branding":      h.branding,
		"RecipientName": "there",
		"ReferenceID":   row.ID,
		"Title":         row.Title,
		"Intent":        row,
	}

	body, subjectOverride := h.renderActiveTemplateByKey("giving_intent_confirmation", data)
	if subjectOverride != "" {
		subject = subjectOverride
	}
	if strings.TrimSpace(body) == "" {
		body = email.RenderGivingIntentConfirmationEmail(email.GivingIntentConfirmationTemplateData{
			Branding:      h.branding,
			RecipientName: "there",
			ReferenceID:   row.ID,
			Title:         row.Title,
		})
	}

	if err := h.sender.SendHTML(addr, subject, body); err != nil {
		applog.L().Warn("failed to send giving confirmation", "to", addr, "error", err)
	}
}

func (h *EngagementHandler) renderActiveTemplateByKey(templateKey string, data any) (body string, subject string) {
	if h.tplRepo == nil {
		return "", ""
	}
	tpl, err := h.tplRepo.GetActiveByKey(strings.TrimSpace(templateKey))
	if err != nil || tpl == nil || strings.TrimSpace(tpl.HTMLBody) == "" {
		return "", ""
	}

	compiled, err := htmltemplate.New("engagement").Option("missingkey=zero").Parse(tpl.HTMLBody)
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

func extractMetadataEmail(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	if v, ok := meta["email"]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
