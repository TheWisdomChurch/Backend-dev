package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type AdminEmailService interface {
	SendComposeEmail(req *models.SendAdminComposeEmailRequest, actor *models.AdminEmailSendActor) (*models.SendAdminComposeEmailResponse, error)
	ListDeliveries(page, limit int) ([]models.AdminEmailDeliveryHistoryItem, int64, error)
}

type adminEmailService struct {
	formRepo        repository.FormRepository
	templateRepo    repository.EmailTemplateRepository
	deliveryRepo    repository.AdminEmailDeliveryRepository
	sender          EmailSender
	branding        email.Branding
	tplStore        *email.TemplateStore
	templateTimeout time.Duration
}

type adminComposeRequestView struct {
	Subject          string
	HTMLBody         string
	TextBody         string
	TemplateID       string
	TemplateKey      string
	ManualRecipients []adminComposeRecipient
	FormIDs          []string
}

type adminComposeRecipient struct {
	Email string
	Name  string
}

type adminComposeFormAudience struct {
	Summary    models.AdminEmailAudienceFormSummary
	Recipients []adminComposeRecipient
	Skipped    int
}

func NewAdminEmailService(
	formRepo repository.FormRepository,
	templateRepo repository.EmailTemplateRepository,
	deliveryRepo repository.AdminEmailDeliveryRepository,
	sender EmailSender,
	branding email.Branding,
) AdminEmailService {
	var tplStore *email.TemplateStore
	if strings.TrimSpace(os.Getenv("SPACES_PUBLIC_BASE_URL")) != "" {
		if ts, err := email.NewTemplateStoreFromEnv(); err == nil {
			tplStore = ts
		}
	}

	templateTimeout := 8 * time.Second
	if raw := strings.TrimSpace(os.Getenv("EMAIL_TEMPLATE_REMOTE_TIMEOUT_SECONDS")); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			templateTimeout = time.Duration(sec) * time.Second
		}
	}

	return &adminEmailService{
		formRepo:        formRepo,
		templateRepo:    templateRepo,
		deliveryRepo:    deliveryRepo,
		sender:          sender,
		branding:        branding,
		tplStore:        tplStore,
		templateTimeout: templateTimeout,
	}
}

func (s *adminEmailService) ListDeliveries(page, limit int) ([]models.AdminEmailDeliveryHistoryItem, int64, error) {
	if s.deliveryRepo == nil {
		return nil, 0, errors.New("admin email delivery repository is not configured")
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	rows, total, err := s.deliveryRepo.List((page-1)*limit, limit)
	if err != nil {
		return nil, 0, err
	}

	items := make([]models.AdminEmailDeliveryHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, models.AdminEmailDeliveryHistoryItem{
			ID:               row.ID,
			Subject:          row.Subject,
			TemplateSource:   row.TemplateSource,
			TemplateID:       row.TemplateID,
			TemplateKey:      row.TemplateKey,
			AudienceSource:   string(row.AudienceSource),
			ManualRecipients: row.ManualRecipients,
			FormRecipients:   row.FormRecipients,
			SourceForms:      decodeAdminEmailSourceFormsJSON(row.SourceForms),
			Status:           string(row.Status),
			TotalRecipients:  row.TotalRecipients,
			Targeted:         row.Targeted,
			Sent:             row.Sent,
			Skipped:          row.Skipped,
			Failed:           row.Failed,
			FailedRecipients: decodeStringListJSON(row.FailedRecipients),
			StartedAt:        row.StartedAt.UTC().Format(time.RFC3339),
			CompletedAt:      formatOptionalTimeRFC3339(row.CompletedAt),
			CreatedByUserID:  row.CreatedByUserID,
			CreatedByEmail:   row.CreatedByEmail,
			CreatedByRole:    row.CreatedByRole,
			CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}

	return items, total, nil
}

func (s *adminEmailService) SendComposeEmail(req *models.SendAdminComposeEmailRequest, actor *models.AdminEmailSendActor) (*models.SendAdminComposeEmailResponse, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if senderStatus, ok := s.sender.(interface{ DisabledReason() string }); ok {
		if reason := strings.TrimSpace(senderStatus.DisabledReason()); reason != "" {
			return nil, errors.New(reason)
		}
	}
	if s.formRepo == nil {
		return nil, errors.New("form repository is not configured")
	}
	if s.deliveryRepo == nil {
		return nil, errors.New("admin email delivery repository is not configured")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}

	normalized, err := normalizeAdminComposeRequest(req)
	if err != nil {
		return nil, err
	}

	templateSelection, err := s.resolveComposeTemplate(normalized.TemplateID, normalized.TemplateKey)
	if err != nil {
		return nil, err
	}
	if templateSelection == nil && strings.TrimSpace(normalized.HTMLBody) != "" {
		var textBody *string
		if strings.TrimSpace(normalized.TextBody) != "" {
			value := normalized.TextBody
			textBody = &value
		}
		templateSelection = &formCampaignTemplateSelection{
			DBTemplate: &models.EmailTemplate{
				HTMLBody: normalized.HTMLBody,
				TextBody: textBody,
			},
			Source: "request_html",
		}
	}
	if templateSelection == nil {
		return nil, errors.New("templateId, templateKey, or htmlBody is required")
	}

	subject := strings.TrimSpace(normalized.Subject)
	if subject == "" && templateSelection.DBTemplate != nil && templateSelection.DBTemplate.Subject != nil {
		subject = strings.TrimSpace(*templateSelection.DBTemplate.Subject)
	}
	if subject == "" {
		subject = "Important update from Wisdom Church"
	}

	startedAt := time.Now().UTC()
	templateSource := strings.TrimSpace(templateSelection.Source)
	if templateSource == "" {
		templateSource = "request_html"
	}

	resp := &models.SendAdminComposeEmailResponse{
		Subject:          subject,
		TemplateSource:   templateSource,
		FailedRecipients: []string{},
		StartedAt:        startedAt.Format(time.RFC3339),
		SentAt:           startedAt.Format(time.RFC3339),
	}

	resolvedRecipients := make(map[string]adminComposeRecipient)
	resp.ManualRecipients = len(normalized.ManualRecipients)
	for _, recipient := range normalized.ManualRecipients {
		resolvedRecipients[recipient.Email] = recipient
	}

	sourceForms := make([]models.AdminEmailAudienceFormSummary, 0, len(normalized.FormIDs))
	for _, formID := range normalized.FormIDs {
		audience, err := s.resolveFormAudience(formID)
		if err != nil {
			return nil, err
		}
		resp.FormRecipients += audience.Summary.UniqueRecipients
		resp.Skipped += audience.Skipped
		sourceForms = append(sourceForms, audience.Summary)

		for _, recipient := range audience.Recipients {
			if existing, exists := resolvedRecipients[recipient.Email]; exists {
				if existing.Name == "" && recipient.Name != "" {
					existing.Name = recipient.Name
					resolvedRecipients[recipient.Email] = existing
				}
				resp.Skipped++
				continue
			}
			resolvedRecipients[recipient.Email] = recipient
		}
	}
	resp.SourceForms = sourceForms
	resp.AudienceSource = string(deriveAdminEmailAudienceSource(resp.ManualRecipients, resp.FormRecipients))

	if len(resolvedRecipients) == 0 {
		return nil, errors.New("no valid recipients were resolved from the selected audience")
	}

	resp.Targeted = len(resolvedRecipients)
	resp.TotalRecipients = resp.Targeted

	for _, recipient := range resolvedRecipients {
		subscribeURL, unsubscribeURL := buildAdminComposeSubscriptionLinks(s.branding, recipient.Email, recipient.Name)
		recipientName := strings.TrimSpace(recipient.Name)
		if recipientName == "" {
			recipientName = firstToken(strings.TrimSpace(strings.SplitN(recipient.Email, "@", 2)[0]))
		}
		if recipientName == "" {
			recipientName = "Friend"
		}
		templateData := map[string]any{
			"Branding":             s.branding,
			"RecipientName":        recipientName,
			"FullName":             recipientName,
			"Name":                 recipientName,
			"FirstName":            firstToken(recipientName),
			"Email":                recipient.Email,
			"Subject":              subject,
			"SubscribeURL":         subscribeURL,
			"UnsubscribeURL":       unsubscribeURL,
			"AudienceSource":       resp.AudienceSource,
			"ManualRecipientCount": resp.ManualRecipients,
			"FormRecipientCount":   resp.FormRecipients,
			"TotalRecipientCount":  resp.Targeted,
			"SourceForms":          sourceForms,
			"Year":                 time.Now().UTC().Year(),
		}

		content, err := s.renderComposeContent(templateSelection, templateData)
		if err != nil {
			resp.Failed++
			resp.FailedRecipients = appendFailedRecipient(resp.FailedRecipients, recipient.Email)
			continue
		}

		if err := sendRenderedAdminEmail(s.sender, recipient.Email, subject, content); err != nil {
			resp.Failed++
			resp.FailedRecipients = appendFailedRecipient(resp.FailedRecipients, recipient.Email)
			continue
		}

		resp.Sent++
	}

	resp.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	resp.SentAt = resp.CompletedAt

	delivery, saveErr := s.saveAdminEmailDelivery(templateSelection, resp, actor)
	if saveErr != nil {
		log.Printf("⚠️ failed to persist admin email delivery: %v", saveErr)
		return resp, nil
	}
	if delivery != nil {
		resp.DeliveryID = &delivery.ID
	}

	return resp, nil
}

func normalizeAdminComposeRequest(req *models.SendAdminComposeEmailRequest) (*adminComposeRequestView, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}

	subject, err := normalizeFormCampaignText("subject", req.Subject, 180)
	if err != nil {
		return nil, err
	}

	templateID := strings.TrimSpace(valueOrEmpty(req.TemplateID))
	if len(templateID) > 120 {
		return nil, errors.New("templateId is too long")
	}

	templateKey := strings.Trim(strings.TrimSpace(valueOrEmpty(req.TemplateKey)), "/")
	if templateKey != "" {
		if strings.Contains(templateKey, "..") || !templateKeyRe.MatchString(templateKey) {
			return nil, errors.New("templateKey contains invalid characters")
		}
	}
	if templateID != "" && templateKey != "" {
		return nil, errors.New("templateId and templateKey cannot be used together")
	}

	htmlBody := strings.TrimSpace(valueOrEmpty(req.HTMLBody))
	textBody := strings.TrimSpace(valueOrEmpty(req.TextBody))

	manualRecipients, err := normalizeAdminComposeRecipients(req.ManualRecipients)
	if err != nil {
		return nil, err
	}
	formIDs := normalizeAdminComposeFormIDs(req.FormIDs)

	if len(manualRecipients) == 0 && len(formIDs) == 0 {
		return nil, errors.New("select at least one form audience or add at least one manual recipient")
	}
	if templateID == "" && templateKey == "" && htmlBody == "" {
		return nil, errors.New("htmlBody, templateId, or templateKey is required")
	}

	return &adminComposeRequestView{
		Subject:          subject,
		HTMLBody:         htmlBody,
		TextBody:         textBody,
		TemplateID:       templateID,
		TemplateKey:      templateKey,
		ManualRecipients: manualRecipients,
		FormIDs:          formIDs,
	}, nil
}

func normalizeAdminComposeRecipients(items *[]models.AdminEmailRecipientInput) ([]adminComposeRecipient, error) {
	if items == nil {
		return nil, nil
	}

	recipients := make([]adminComposeRecipient, 0, len(*items))
	seen := make(map[string]struct{}, len(*items))
	for i, item := range *items {
		emailAddr := normalizeEmail(item.Email)
		name := strings.TrimSpace(valueOrEmpty(item.Name))
		if emailAddr == "" && name == "" {
			continue
		}
		if emailAddr == "" {
			return nil, fmt.Errorf("manualRecipients[%d].email is required", i)
		}
		if !emailRe.MatchString(emailAddr) {
			return nil, fmt.Errorf("manualRecipients[%d].email must be a valid email", i)
		}
		if utf8.RuneCountInString(name) > 120 {
			return nil, fmt.Errorf("manualRecipients[%d].name must be 120 characters or fewer", i)
		}
		if _, exists := seen[emailAddr]; exists {
			continue
		}
		seen[emailAddr] = struct{}{}
		recipients = append(recipients, adminComposeRecipient{
			Email: emailAddr,
			Name:  name,
		})
	}
	return recipients, nil
}

func normalizeAdminComposeFormIDs(values *[]string) []string {
	if values == nil {
		return nil
	}
	formIDs := make([]string, 0, len(*values))
	seen := make(map[string]struct{}, len(*values))
	for _, raw := range *values {
		formID := strings.TrimSpace(raw)
		if formID == "" {
			continue
		}
		if _, exists := seen[formID]; exists {
			continue
		}
		seen[formID] = struct{}{}
		formIDs = append(formIDs, formID)
	}
	return formIDs
}

func (s *adminEmailService) resolveComposeTemplate(templateID, templateKey string) (*formCampaignTemplateSelection, error) {
	if templateID == "" && templateKey == "" {
		return nil, nil
	}

	if templateID != "" {
		if s.templateRepo == nil {
			return nil, errors.New("email template repository is not configured")
		}
		tpl, err := s.templateRepo.GetByID(templateID)
		if err != nil {
			return nil, err
		}
		if tpl.Status == models.EmailTemplateArchived {
			return nil, errors.New("selected email template is archived")
		}
		return &formCampaignTemplateSelection{
			DBTemplate: tpl,
			Source:     "db:" + tpl.ID,
		}, nil
	}

	if s.templateRepo != nil {
		if tpl, err := s.templateRepo.GetActiveByKey(templateKey); err == nil && tpl != nil {
			if tpl.Status == models.EmailTemplateArchived {
				return nil, errors.New("selected email template is archived")
			}
			return &formCampaignTemplateSelection{
				DBTemplate: tpl,
				Source:     "db_key:" + templateKey,
			}, nil
		}
	}
	if s.tplStore != nil {
		return &formCampaignTemplateSelection{
			RemoteKey: templateKey,
			Source:    "remote:" + templateKey,
		}, nil
	}

	return nil, fmt.Errorf("email template %q was not found", templateKey)
}

func (s *adminEmailService) resolveFormAudience(formID string) (*adminComposeFormAudience, error) {
	form, err := s.formRepo.GetByID(strings.TrimSpace(formID))
	if err != nil {
		return nil, err
	}

	submissions, err := s.formRepo.ListEmailSubmissions(form.ID)
	if err != nil {
		return nil, err
	}

	recipients := make([]adminComposeRecipient, 0, len(submissions))
	seen := make(map[string]struct{}, len(submissions))
	validRecipients := 0
	skipped := 0
	for _, submission := range submissions {
		emailAddr := normalizeEmail(valueOrEmpty(submission.Email))
		if emailAddr == "" || !emailRe.MatchString(emailAddr) {
			skipped++
			continue
		}
		validRecipients++
		if _, exists := seen[emailAddr]; exists {
			skipped++
			continue
		}
		seen[emailAddr] = struct{}{}

		name := strings.TrimSpace(valueOrEmpty(submission.Name))
		if name == "" {
			values := decodeSubmissionValues(submission.Values)
			name = strings.TrimSpace(valueAsString(values, "fullName", "full_name", "name", "firstName", "first_name"))
		}

		recipients = append(recipients, adminComposeRecipient{
			Email: emailAddr,
			Name:  name,
		})
	}

	return &adminComposeFormAudience{
		Summary: models.AdminEmailAudienceFormSummary{
			FormID:           form.ID,
			FormTitle:        strings.TrimSpace(form.Title),
			TotalSubmissions: len(submissions),
			ValidRecipients:  validRecipients,
			UniqueRecipients: len(recipients),
		},
		Recipients: recipients,
		Skipped:    skipped,
	}, nil
}

func deriveAdminEmailAudienceSource(manualCount, formCount int) models.AdminEmailAudienceSource {
	switch {
	case manualCount > 0 && formCount > 0:
		return models.AdminEmailAudienceMixed
	case formCount > 0:
		return models.AdminEmailAudienceForms
	default:
		return models.AdminEmailAudienceManual
	}
}

func buildAdminComposeSubscriptionLinks(branding email.Branding, emailAddr, recipientName string) (string, string) {
	base := strings.TrimRight(strings.TrimSpace(branding.PublicURL), "/")
	addr := strings.TrimSpace(emailAddr)
	if base == "" || addr == "" {
		return "", ""
	}

	subscribeURL := base + "/api/v1/notifications/subscribe?email=" + urlQueryEscape(addr)
	if strings.TrimSpace(recipientName) != "" {
		subscribeURL += "&name=" + urlQueryEscape(recipientName)
	}
	unsubscribeURL := base + "/api/v1/notifications/unsubscribe?email=" + urlQueryEscape(addr)
	return subscribeURL, unsubscribeURL
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func (s *adminEmailService) renderComposeContent(selection *formCampaignTemplateSelection, templateData map[string]any) (*formCampaignRenderedContent, error) {
	if selection == nil {
		return nil, errors.New("template selection is required")
	}
	if selection.DBTemplate != nil {
		htmlBody, err := renderDBTemplate(selection.DBTemplate, templateData)
		if err != nil {
			return nil, err
		}
		textBody, err := renderDBTextTemplate(selection.DBTemplate, templateData)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(textBody) == "" {
			textBody = stripHTMLToText(htmlBody)
		}
		return &formCampaignRenderedContent{
			HTML: htmlBody,
			Text: textBody,
		}, nil
	}

	if selection.RemoteKey != "" && s.tplStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.templateTimeout)
		defer cancel()

		textOut, htmlOut, _, err := s.tplStore.RenderWithData(ctx, selection.RemoteKey, templateData)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(htmlOut) == "" {
			return nil, errors.New("remote template rendered empty body")
		}
		if strings.TrimSpace(textOut) == "" {
			textOut = stripHTMLToText(htmlOut)
		}
		return &formCampaignRenderedContent{
			HTML: htmlOut,
			Text: textOut,
		}, nil
	}

	return nil, errors.New("email template could not be rendered")
}

func sendRenderedAdminEmail(sender EmailSender, to, subject string, content *formCampaignRenderedContent) error {
	if sender == nil {
		return errors.New("email sender is not configured")
	}
	if content == nil {
		return errors.New("rendered email content is nil")
	}

	htmlBody := strings.TrimSpace(content.HTML)
	if htmlBody == "" {
		return errors.New("rendered email html body is empty")
	}
	textBody := strings.TrimSpace(content.Text)

	if multipart, ok := sender.(interface {
		SendHTMLText(to, subject, htmlBody, textBody string) error
	}); ok && textBody != "" {
		return multipart.SendHTMLText(to, subject, htmlBody, textBody)
	}

	return sender.SendHTML(to, subject, htmlBody)
}

func (s *adminEmailService) saveAdminEmailDelivery(
	selection *formCampaignTemplateSelection,
	resp *models.SendAdminComposeEmailResponse,
	actor *models.AdminEmailSendActor,
) (*models.AdminEmailDelivery, error) {
	if resp == nil {
		return nil, errors.New("admin email delivery payload is incomplete")
	}

	startedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(resp.StartedAt))
	if err != nil {
		return nil, err
	}

	var completedAt *time.Time
	if value := strings.TrimSpace(resp.CompletedAt); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return nil, parseErr
		}
		completedAt = &parsed
	}

	templateID, templateKey := extractFormCampaignTemplateReference(selection)
	delivery := &models.AdminEmailDelivery{
		Subject:          strings.TrimSpace(resp.Subject),
		TemplateSource:   strings.TrimSpace(resp.TemplateSource),
		TemplateID:       templateID,
		TemplateKey:      templateKey,
		AudienceSource:   models.AdminEmailAudienceSource(strings.TrimSpace(resp.AudienceSource)),
		ManualRecipients: resp.ManualRecipients,
		FormRecipients:   resp.FormRecipients,
		SourceForms:      encodeAdminEmailSourceFormsJSON(resp.SourceForms),
		Status:           deriveAdminEmailDeliveryStatus(resp),
		TotalRecipients:  resp.TotalRecipients,
		Targeted:         resp.Targeted,
		Sent:             resp.Sent,
		Skipped:          resp.Skipped,
		Failed:           resp.Failed,
		FailedRecipients: encodeStringListJSON(resp.FailedRecipients),
		StartedAt:        startedAt.UTC(),
		CompletedAt:      completedAt,
	}

	applyAdminEmailDeliveryActor(delivery, actor)

	if err := s.deliveryRepo.Create(delivery); err != nil {
		return nil, err
	}
	return delivery, nil
}

func applyAdminEmailDeliveryActor(delivery *models.AdminEmailDelivery, actor *models.AdminEmailSendActor) {
	if delivery == nil || actor == nil {
		return
	}
	if value := strings.TrimSpace(actor.UserID); value != "" {
		delivery.CreatedByUserID = &value
	}
	if value := strings.TrimSpace(actor.Email); value != "" {
		delivery.CreatedByEmail = &value
	}
	if value := strings.TrimSpace(actor.Role); value != "" {
		delivery.CreatedByRole = &value
	}
}

func deriveAdminEmailDeliveryStatus(resp *models.SendAdminComposeEmailResponse) models.AdminEmailDeliveryStatus {
	if resp == nil {
		return models.AdminEmailDeliveryFailed
	}
	switch {
	case resp.Failed <= 0:
		return models.AdminEmailDeliveryCompleted
	case resp.Sent > 0:
		return models.AdminEmailDeliveryPartial
	default:
		return models.AdminEmailDeliveryFailed
	}
}

func encodeAdminEmailSourceFormsJSON(items []models.AdminEmailAudienceFormSummary) datatypes.JSON {
	if len(items) == 0 {
		return datatypes.JSON([]byte("[]"))
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return datatypes.JSON([]byte("[]"))
	}
	return datatypes.JSON(raw)
}

func decodeAdminEmailSourceFormsJSON(raw datatypes.JSON) []models.AdminEmailAudienceFormSummary {
	if len(raw) == 0 || string(raw) == "null" {
		return []models.AdminEmailAudienceFormSummary{}
	}
	var items []models.AdminEmailAudienceFormSummary
	if err := json.Unmarshal(raw, &items); err != nil {
		return []models.AdminEmailAudienceFormSummary{}
	}
	return items
}
