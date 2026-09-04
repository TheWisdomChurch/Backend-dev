// internal/service/form_service_public.go
package service

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/exportpdf"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
)

func (s *formService) GetPublic(slug string) (*models.PublicFormPayload, error) {
	form, err := s.repo.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	form.Fields = applyImplicitFieldVisibilityDefaults(form.Fields)
	s.attachPublicURL(form)

	settings, _ := decodeSettings(form.Settings)
	if settings.ExpiresAt != nil && strings.TrimSpace(*settings.ExpiresAt) != "" {
		t, parseErr := parseFlexibleTime(*settings.ExpiresAt)
		if parseErr != nil {
			return nil, errors.New("form expiresAt is invalid on server")
		}
		if time.Now().After(t) {
			return nil, ErrFormExpired
		}
	}
	if settings.ClosesAt != nil && strings.TrimSpace(*settings.ClosesAt) != "" {
		t, parseErr := parseFlexibleTime(*settings.ClosesAt)
		if parseErr != nil {
			return nil, errors.New("form closesAt is invalid on server")
		}
		if time.Now().After(t) {
			return nil, ErrFormClosed
		}
	}

	payload := &models.PublicFormPayload{Form: form}

	// Optional embed event
	if form.EventID != nil && s.eventRepo != nil {
		ev, evErr := s.eventRepo.GetByID(*form.EventID)
		if evErr == nil {
			payload.Event = ev
		}
	}

	return payload, nil
}

func (s *formService) GetPublicReport(slug, accessToken string, page, limit int, start, end *time.Time) (*models.PublicFormReportPayload, error) {
	form, err := s.getAuthorizedPublicReportForm(slug, accessToken)
	if err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}

	offset := (page - 1) * limit
	items, total, err := s.repo.ListSubmissions(form.ID, offset, limit, start, end)
	if err != nil {
		return nil, err
	}

	latestItems, _, err := s.repo.ListSubmissions(form.ID, 0, 10, start, end)
	if err != nil {
		return nil, err
	}

	orderedFields := orderFormFields(form.Fields)
	token := strings.TrimSpace(accessToken)
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	reportURL := s.buildPublicAPIURL(s.buildPublicReportPath(strings.TrimSpace(ptrString(form.Slug)), token))
	reportDataURL := s.buildPublicAPIURL(s.buildPublicReportDataPath(strings.TrimSpace(ptrString(form.Slug)), token))
	exportPDFURL := s.buildPublicAPIURL(s.buildPublicReportPDFPath(strings.TrimSpace(ptrString(form.Slug)), token))

	return &models.PublicFormReportPayload{
		BrandName:           s.reportBrandName(),
		FormID:              form.ID,
		FormTitle:           strings.TrimSpace(form.Title),
		FormDescription:     form.Description,
		Slug:                strings.TrimSpace(ptrString(form.Slug)),
		Summary:             buildFormReportSummary(latestItems, total),
		LatestRegistrations: buildReportSubmissionList(latestItems, orderedFields),
		Submissions:         buildReportSubmissionList(items, orderedFields),
		Page:                page,
		Limit:               limit,
		Total:               total,
		TotalPages:          totalPages,
		ReportURL:           reportURL,
		ReportDataURL:       reportDataURL,
		ExportPDFURL:        exportPDFURL,
		GeneratedAt:         time.Now().UTC(),
	}, nil
}

func (s *formService) Submit(slug string, req *models.SubmitFormRequest) error {
	form, err := s.repo.GetBySlug(slug)
	if err != nil {
		return err
	}

	settings, _ := decodeSettings(form.Settings)
	consentAccepted, _ := req.Values["_consentAccepted"].(bool)
	if settings.Consent != nil && settings.Consent.Required != nil && *settings.Consent.Required && !consentAccepted {
		return errors.New("you must accept the consent and privacy notice before submitting")
	}

	if settings.ExpiresAt != nil && strings.TrimSpace(*settings.ExpiresAt) != "" {
		t, parseErr := parseFlexibleTime(*settings.ExpiresAt)
		if parseErr != nil {
			return errors.New("form expiresAt is invalid on server")
		}
		if time.Now().After(t) {
			return ErrFormExpired
		}
	}

	if settings.ClosesAt != nil && strings.TrimSpace(*settings.ClosesAt) != "" {
		t, parseErr := parseFlexibleTime(*settings.ClosesAt)
		if parseErr != nil {
			return errors.New("form closesAt is invalid on server")
		}
		if time.Now().After(t) {
			return ErrFormClosed
		}
	}

	if settings.Capacity != nil && *settings.Capacity > 0 {
		count, err := s.repo.CountSubmissions(form.ID)
		if err != nil {
			return err
		}
		if int(count) >= *settings.Capacity {
			return errors.New("registration full")
		}
	}

	fields := applyImplicitFieldVisibilityDefaults(form.Fields)

	fieldValues := submissionFieldValues(req.Values)

	mediaSafeValues, err := s.materializeSubmissionMedia(form, fieldValues)
	if err != nil {
		return err
	}

	cleanValues, err := validateSubmission(fields, mediaSafeValues)
	if err != nil {
		return err
	}
	if consentAccepted {
		cleanValues["_consentAccepted"] = true
		if settings.Consent != nil && settings.Consent.Version != nil {
			cleanValues["_consentVersion"] = strings.TrimSpace(*settings.Consent.Version)
		}
		cleanValues["_consentRecordedAt"] = time.Now().UTC().Format(time.RFC3339)
	}

	if containsSubmissionDataURL(cleanValues) {
		return fmt.Errorf("submission contains embedded base64 media; upload failed before save")
	}
	if len(cleanValues) == 0 {
		return errors.New("at least one field is required")
	}

	// Extract common fields into columns for analytics
	name, email, phone, addr := extractCommonFields(fields, cleanValues)
	if email == nil || strings.TrimSpace(*email) == "" {
		return errors.New("an email address is required so we can send your confirmation")
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		if _, exists := cleanValues["fullName"]; !exists {
			cleanValues["fullName"] = strings.TrimSpace(*name)
		}
		if _, exists := cleanValues["name"]; !exists {
			cleanValues["name"] = strings.TrimSpace(*name)
		}
	}
	if _, exists := cleanValues["email"]; !exists {
		cleanValues["email"] = strings.TrimSpace(*email)
	}
	if phone != nil && strings.TrimSpace(*phone) != "" {
		if _, exists := cleanValues["phone"]; !exists {
			cleanValues["phone"] = strings.TrimSpace(*phone)
		}
	}
	if addr != nil && strings.TrimSpace(*addr) != "" {
		if _, exists := cleanValues["address"]; !exists {
			cleanValues["address"] = strings.TrimSpace(*addr)
		}
	}

	valuesJSON, err := json.Marshal(cleanValues)
	if err != nil {
		return errors.New("failed to store submission")
	}

	var regCode *string
	if s.sequenceRepo != nil {
		if code, err := s.buildRegistrationCode(form); err == nil && code != "" {
			regCode = &code
		}
	}

	sub := &models.FormSubmission{
		FormID:           form.ID,
		Name:             name,
		Email:            email,
		ContactNumber:    phone,
		ContactAddress:   addr,
		RegistrationCode: regCode,
		Values:           datatypes.JSON(valuesJSON),
	}

	if err := s.repo.CreateSubmission(sub); err != nil {
		return err
	}

	s.sendResponseEmail(form, settings, cleanValues, name, *email, regCode, sub.ID)
	if err := s.syncSubmissionTarget(form, settings, cleanValues, sub.ID); err != nil {
		target := resolveSubmissionTargetForForm(form, settings)
		applog.L().Warn("submission target sync failed",
			"form_id", strings.TrimSpace(form.ID),
			"slug", strings.TrimSpace(valueOrEmpty(form.Slug)),
			"submission_id", strings.TrimSpace(sub.ID),
			"target", target,
			"error", err,
		)
		s.notifySubmissionTargetSyncFailure(form, sub.ID, target, err)
	}

	return nil
}

// submissionFieldValues separates server-managed consent metadata from answers
// before answers are checked against the form's configured field keys.
func submissionFieldValues(values map[string]any) map[string]any {
	fieldValues := make(map[string]any, len(values))
	for key, value := range values {
		if key == "_consentAccepted" || key == "_consentVersion" {
			continue
		}
		fieldValues[key] = value
	}
	return fieldValues
}

func (s *formService) BuildAdminReportPDF(formID string, start, end *time.Time) (string, []byte, error) {
	form, err := s.repo.GetByID(formID)
	if err != nil {
		return "", nil, err
	}
	return s.buildFormReportPDF(form, start, end)
}

func (s *formService) BuildPublicReportPDF(slug, accessToken string, start, end *time.Time) (string, []byte, error) {
	form, err := s.getAuthorizedPublicReportForm(slug, accessToken)
	if err != nil {
		return "", nil, err
	}
	return s.buildFormReportPDF(form, start, end)
}

func (s *formService) ListSubmissions(formID string, page, limit int, start, end *time.Time) ([]models.FormSubmission, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.ListSubmissions(formID, offset, limit, start, end)
}

func (s *formService) DeleteSubmission(id string) error {
	return s.repo.DeleteSubmission(id)
}

func (s *formService) ListCampaignDeliveries(formID string, page, limit int) ([]models.FormCampaignDeliveryHistoryItem, int64, error) {
	if strings.TrimSpace(formID) == "" {
		return nil, 0, errors.New("form id is required")
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	if _, err := s.repo.GetByID(strings.TrimSpace(formID)); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	rows, total, err := s.repo.ListCampaignDeliveries(strings.TrimSpace(formID), offset, limit)
	if err != nil {
		return nil, 0, err
	}

	items := make([]models.FormCampaignDeliveryHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, models.FormCampaignDeliveryHistoryItem{
			ID:               row.ID,
			FormID:           row.FormID,
			FormTitle:        row.FormTitle,
			EventID:          row.EventID,
			EventTitle:       row.EventTitle,
			Subject:          row.Subject,
			TemplateSource:   row.TemplateSource,
			TemplateID:       row.TemplateID,
			TemplateKey:      row.TemplateKey,
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

func (s *formService) reportBrandName() string {
	brandName := strings.TrimSpace(s.branding.AppName)
	if brandName == "" {
		brandName = "Submission Report"
	}
	return brandName
}

func (s *formService) getAuthorizedPublicReportForm(slug, accessToken string) (*models.Form, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, ErrFormReportAccessDenied
	}

	form, err := s.repo.GetBySlug(slug)
	if err != nil {
		return nil, err
	}

	expected := strings.TrimSpace(ptrString(form.ReportAccessToken))
	if expected == "" {
		return nil, ErrFormReportAccessDenied
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(token)) != 1 {
		return nil, ErrFormReportAccessDenied
	}

	return form, nil
}

func (s *formService) buildFormReportPDF(form *models.Form, start, end *time.Time) (string, []byte, error) {
	if form == nil {
		return "", nil, errors.New("form not found")
	}

	allSubmissions, err := s.listAllSubmissionsForReport(form.ID, start, end)
	if err != nil {
		return "", nil, err
	}

	labelMap := make(map[string]string, len(form.Fields))
	for _, field := range orderFormFields(form.Fields) {
		key := strings.TrimSpace(field.Key)
		label := strings.TrimSpace(field.Label)
		if key == "" {
			continue
		}
		if label == "" {
			label = humanizeFieldKey(key)
		}
		labelMap[key] = label
	}

	subs := make([]exportpdf.Submission, 0, len(allSubmissions))
	for _, item := range allSubmissions {
		subs = append(subs, exportpdf.Submission{
			ID:               item.ID,
			Name:             ptrString(item.Name),
			Email:            ptrString(item.Email),
			ContactNumber:    ptrString(item.ContactNumber),
			ContactAddress:   ptrString(item.ContactAddress),
			RegistrationCode: ptrString(item.RegistrationCode),
			CreatedAt:        item.CreatedAt,
			Values:           decodeSubmissionValues(item.Values),
			FieldLabels:      labelMap,
		})
	}

	fileName := reportFileName(form.Title)
	pdfBytes, err := exportpdf.BuildSubmissionsPDF(s.reportBrandName(), strings.TrimSpace(form.Title), subs)
	if err != nil {
		return "", nil, err
	}

	return fileName, pdfBytes, nil
}

func (s *formService) listAllSubmissionsForReport(formID string, start, end *time.Time) ([]models.FormSubmission, error) {
	const pageSize = 250

	offset := 0
	all := make([]models.FormSubmission, 0, pageSize)

	for {
		items, total, err := s.repo.ListSubmissions(formID, offset, pageSize, start, end)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		offset += len(items)

		if int64(len(all)) >= total || len(items) < pageSize {
			break
		}
	}

	return all, nil
}

func buildFormReportSummary(latest []models.FormSubmission, total int64) models.FormReportSummary {
	summary := models.FormReportSummary{
		TotalSubmissions: total,
	}
	if len(latest) > 0 {
		latestAt := latest[0].CreatedAt
		summary.LatestSubmissionAt = &latestAt
	}
	return summary
}

func buildReportSubmissionList(items []models.FormSubmission, orderedFields []models.FormField) []models.FormReportSubmission {
	out := make([]models.FormReportSubmission, 0, len(items))
	for _, item := range items {
		out = append(out, buildReportSubmission(item, orderedFields))
	}
	return out
}

func buildReportSubmission(item models.FormSubmission, orderedFields []models.FormField) models.FormReportSubmission {
	values := decodeSubmissionValues(item.Values)
	fields := make([]models.FormReportFieldValue, 0, len(values))
	seen := make(map[string]bool, len(values))

	for _, field := range orderedFields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}
		value, ok := values[key]
		if !ok {
			continue
		}
		text := reportValueString(value)
		if text == "" {
			continue
		}
		label := strings.TrimSpace(field.Label)
		if label == "" {
			label = humanizeFieldKey(key)
		}
		fields = append(fields, models.FormReportFieldValue{
			Key:   key,
			Label: label,
			Value: text,
		})
		seen[key] = true
	}

	extras := make([]string, 0)
	for key := range values {
		if seen[key] {
			continue
		}
		extras = append(extras, key)
	}
	sort.Strings(extras)
	for _, key := range extras {
		text := reportValueString(values[key])
		if text == "" {
			continue
		}
		fields = append(fields, models.FormReportFieldValue{
			Key:   key,
			Label: humanizeFieldKey(key),
			Value: text,
		})
	}

	return models.FormReportSubmission{
		ID:               item.ID,
		Name:             item.Name,
		Email:            item.Email,
		ContactNumber:    item.ContactNumber,
		ContactAddress:   item.ContactAddress,
		RegistrationCode: item.RegistrationCode,
		Values:           values,
		Fields:           fields,
		CreatedAt:        item.CreatedAt,
	}
}

func orderFormFields(fields []models.FormField) []models.FormField {
	ordered := append([]models.FormField(nil), fields...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Order == ordered[j].Order {
			return ordered[i].Key < ordered[j].Key
		}
		return ordered[i].Order < ordered[j].Order
	})
	return ordered
}

func decodeSubmissionValues(raw datatypes.JSON) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}

	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func reportValueString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case bool:
		if v {
			return "Yes"
		}
		return "No"
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		f := float64(v)
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	case []string:
		return strings.Join(v, ", ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			part := reportValueString(item)
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func humanizeFieldKey(key string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(key, "_", " "), "-", " "))
	if clean == "" {
		return "Response"
	}
	parts := strings.Fields(clean)
	for i := range parts {
		runes := []rune(strings.ToLower(parts[i]))
		if len(runes) == 0 {
			continue
		}
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func reportFileName(title string) string {
	t := strings.TrimSpace(title)
	t = strings.ToLower(t)
	t = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == ' ':
			return r
		default:
			return -1
		}
	}, t)
	t = strings.Join(strings.Fields(t), "-")
	if t == "" {
		t = "form"
	}
	return t + "-submissions.pdf"
}
