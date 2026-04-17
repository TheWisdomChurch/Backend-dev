// internal/service/form_service.go
package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/exportpdf"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

var slugInvalidRe = regexp.MustCompile(`[^a-z0-9\-]+`)
var slugDashCollapseRe = regexp.MustCompile(`-+`)
var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var phoneRe = regexp.MustCompile(`^[0-9()+\-\s]{7,20}$`)
var templateKeyRe = regexp.MustCompile(`^[A-Za-z0-9/_-]+$`)
var ErrFormExpired = errors.New("form expired")
var ErrFormClosed = errors.New("registration closed")
var ErrFormReportAccessDenied = errors.New("invalid report link")
var flexibleTimeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

type FormService interface {
	List(page, limit int) ([]models.Form, int64, error)
	GetByID(id string) (*models.Form, error)
	Create(req *models.CreateFormRequest) (*models.Form, error)
	Update(id string, req *models.UpdateFormRequest) (*models.Form, error)
	Delete(id string) error
	Publish(id string) (string, *string, error)
	GetOrCreateReportLink(formID string) (*models.FormReportLinkPayload, error)

	GetPublic(slug string) (*models.PublicFormPayload, error)
	GetPublicReport(slug, accessToken string, page, limit int, start, end *time.Time) (*models.PublicFormReportPayload, error)
	Submit(slug string, req *models.SubmitFormRequest) error
	BuildAdminReportPDF(formID string, start, end *time.Time) (string, []byte, error)
	BuildPublicReportPDF(slug, accessToken string, start, end *time.Time) (string, []byte, error)

	// Admin submissions
	ListSubmissions(formID string, page, limit int, start, end *time.Time) ([]models.FormSubmission, int64, error)
	ListCampaignDeliveries(formID string, page, limit int) ([]models.FormCampaignDeliveryHistoryItem, int64, error)
	Stats(start, end *time.Time) (*models.FormStatsResponse, error)
	StatsByForm(formID string, start, end *time.Time) ([]models.FormSubmissionDailyCount, error)
	CleanupExpiredForms(now time.Time) (int64, error)

	ConfirmCalendarOptIn(slug, token string) (*models.FormCalendarPayload, error)
	BuildCalendarICS(slug, token string) (string, []byte, error)
	SendEventReminderEmails(now time.Time, lookAhead time.Duration) (int, int, error)
	SendFormCampaignEmail(formID string, req *models.SendFormCampaignEmailRequest, actor *models.FormCampaignSendActor) (*models.SendFormCampaignEmailResponse, error)
}

type formService struct {
	repo repository.FormRepository

	// IMPORTANT:
	// In your codebase EventRepository is a concrete type (pointer), not an interface.
	// So keep it as a pointer and nil-check works.
	eventRepo *repository.EventRepository

	sequenceRepo   *repository.RegistrationSequenceRepository
	reminderRepo   repository.FormCalendarReminderRepository
	templateRepo   repository.EmailTemplateRepository
	workforceSvc   WorkforceService
	memberSvc      MemberService
	leadershipSvc  LeadershipService
	testimonialSvc TestimonialService
	sender         EmailSender
	branding       email.Branding

	publicBaseURL   string
	tplStore        *email.TemplateStore
	templateTimeout time.Duration
}

func NewFormService(
	repo repository.FormRepository,
	eventRepo *repository.EventRepository,
	sequenceRepo *repository.RegistrationSequenceRepository,
	reminderRepo repository.FormCalendarReminderRepository,
	templateRepo repository.EmailTemplateRepository,
	workforceSvc WorkforceService,
	memberSvc MemberService,
	leadershipSvc LeadershipService,
	testimonialSvc TestimonialService,
	sender EmailSender,
	branding email.Branding,
	publicBaseURL string,
) FormService {
	var tplStore *email.TemplateStore
	if strings.TrimSpace(os.Getenv("S3_PUBLIC_BASE_URL")) != "" {
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

	return &formService{
		repo:            repo,
		eventRepo:       eventRepo,
		sequenceRepo:    sequenceRepo,
		reminderRepo:    reminderRepo,
		templateRepo:    templateRepo,
		workforceSvc:    workforceSvc,
		memberSvc:       memberSvc,
		leadershipSvc:   leadershipSvc,
		testimonialSvc:  testimonialSvc,
		sender:          sender,
		branding:        branding,
		publicBaseURL:   strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		tplStore:        tplStore,
		templateTimeout: templateTimeout,
	}
}

func (s *formService) buildPublicURL(slug string) *string {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" || s.publicBaseURL == "" {
		return nil
	}
	u := fmt.Sprintf("%s/forms/%s", s.publicBaseURL, slug)
	return &u
}

func (s *formService) attachPublicURL(form *models.Form) {
	if form == nil {
		return
	}
	if !form.IsPublished || form.Slug == nil {
		return
	}
	if form.Status == models.FormStatusInvalid {
		return
	}
	slug := strings.TrimSpace(*form.Slug)
	if slug == "" {
		return
	}
	form.PublicURL = s.buildPublicURL(slug)
}

func (s *formService) attachPublicURLs(forms []models.Form) {
	for i := range forms {
		s.attachPublicURL(&forms[i])
	}
}

func (s *formService) buildPublicAPIURL(path string) string {
	base := strings.TrimRight(strings.TrimSpace(s.branding.PublicURL), "/")
	if base == "" {
		return path
	}
	return base + path
}

func (s *formService) buildPublicReportPath(slug, accessToken string) string {
	return fmt.Sprintf(
		"/reports/forms/%s?access=%s",
		url.PathEscape(strings.TrimSpace(slug)),
		url.QueryEscape(strings.TrimSpace(accessToken)),
	)
}

func (s *formService) buildPublicReportDataPath(slug, accessToken string) string {
	return fmt.Sprintf(
		"/reports/forms/%s/data?access=%s",
		url.PathEscape(strings.TrimSpace(slug)),
		url.QueryEscape(strings.TrimSpace(accessToken)),
	)
}

func (s *formService) buildPublicReportPDFPath(slug, accessToken string) string {
	return fmt.Sprintf(
		"/reports/forms/%s/export.pdf?access=%s",
		url.PathEscape(strings.TrimSpace(slug)),
		url.QueryEscape(strings.TrimSpace(accessToken)),
	)
}

func (s *formService) buildReportLinkPayload(form *models.Form, accessToken string) *models.FormReportLinkPayload {
	if form == nil || form.Slug == nil {
		return nil
	}

	slug := strings.TrimSpace(*form.Slug)
	if slug == "" {
		return nil
	}

	return &models.FormReportLinkPayload{
		FormID:        form.ID,
		FormTitle:     strings.TrimSpace(form.Title),
		Slug:          slug,
		ReportURL:     s.buildPublicAPIURL(s.buildPublicReportPath(slug, accessToken)),
		ReportDataURL: s.buildPublicAPIURL(s.buildPublicReportDataPath(slug, accessToken)),
		ExportPDFURL:  s.buildPublicAPIURL(s.buildPublicReportPDFPath(slug, accessToken)),
	}
}

func (s *formService) ensureReportAccessToken(form *models.Form) (string, error) {
	if form == nil {
		return "", errors.New("form not found")
	}

	if !form.IsPublished || form.Slug == nil || strings.TrimSpace(*form.Slug) == "" {
		return "", errors.New("publish the form before generating a report link")
	}

	if existing := strings.TrimSpace(ptrString(form.ReportAccessToken)); existing != "" {
		return existing, nil
	}

	for attempt := 0; attempt < 3; attempt++ {
		token, err := generateSecureToken(32)
		if err != nil {
			return "", err
		}
		form.ReportAccessToken = &token
		if err := s.repo.Update(form); err != nil {
			if isUniqueViolationErr(err) {
				continue
			}
			return "", err
		}
		return token, nil
	}

	return "", errors.New("failed to generate report link")
}

func (s *formService) List(page, limit int) ([]models.Form, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	items, total, err := s.repo.List(offset, limit)
	if err != nil {
		return nil, 0, err
	}
	s.attachPublicURLs(items)
	return items, total, nil
}

func (s *formService) GetByID(id string) (*models.Form, error) {
	form, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	s.attachPublicURL(form)
	return form, nil
}

func (s *formService) Create(req *models.CreateFormRequest) (*models.Form, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.New("title is required")
	}
	title := strings.TrimSpace(req.Title)

	var normalizedSlug *string
	if req.Slug != nil {
		slug := slugify(*req.Slug)
		if slug != "" {
			exists, err := s.repo.SlugExists(slug)
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, errors.New("slug already in use")
			}
			normalizedSlug = &slug
		}
	}

	settingsJSON, err := encodeSettings(req.Settings)
	if err != nil {
		return nil, err
	}

	form := &models.Form{
		Title:       title,
		Description: req.Description,
		EventID:     req.EventID,
		Slug:        normalizedSlug,
		IsPublished: false,
		Status:      models.FormStatusDraft,
		Settings:    settingsJSON,
	}

	// Validate + build fields (draft allowed; still validate provided fields)
	fields, err := buildAndValidateFields("", req.Fields, true)
	if err != nil {
		return nil, err
	}
	form.Fields = fields

	if err := s.repo.Create(form); err != nil {
		return nil, err
	}

	created, err := s.repo.GetByID(form.ID)
	if err != nil {
		return nil, err
	}
	s.attachPublicURL(created)
	return created, nil
}

func (s *formService) Update(id string, req *models.UpdateFormRequest) (*models.Form, error) {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" {
			return nil, errors.New("title cannot be empty")
		}
		existing.Title = t
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.EventID != nil {
		existing.EventID = req.EventID
	}
	if req.Slug != nil {
		slug := slugify(*req.Slug)
		if slug == "" {
			if existing.Status == models.FormStatusPublished {
				return nil, errors.New("cannot clear slug for a published form")
			}
			existing.Slug = nil
		} else {
			if existing.Status == models.FormStatusPublished && (existing.Slug == nil || *existing.Slug != slug) {
				return nil, errors.New("cannot change slug for a published form")
			}
			if existing.Slug == nil || *existing.Slug != slug {
				exists, err := s.repo.SlugExists(slug)
				if err != nil {
					return nil, err
				}
				if exists {
					return nil, errors.New("slug already in use")
				}
			}
			existing.Slug = &slug
		}
	}
	if req.Settings != nil {
		currentSettings, _ := decodeSettings(existing.Settings)
		mergedSettings, err := mergeFormSettings(currentSettings, req.Settings)
		if err != nil {
			return nil, err
		}
		settingsJSON, err := encodeSettings(mergedSettings)
		if err != nil {
			return nil, err
		}
		existing.Settings = settingsJSON
	}

	// If fields provided => replace
	if req.Fields != nil {
		fields, err := buildAndValidateFields(existing.ID, *req.Fields, false)
		if err != nil {
			return nil, err
		}
		if err := s.repo.ReplaceFields(existing.ID, fields); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Update(existing); err != nil {
		return nil, err
	}

	updated, err := s.repo.GetByID(existing.ID)
	if err != nil {
		return nil, err
	}
	s.attachPublicURL(updated)
	return updated, nil
}

func (s *formService) Delete(id string) error {
	return s.repo.Delete(id)
}

func (s *formService) Publish(id string) (string, *string, error) {
	form, err := s.repo.GetByID(id)
	if err != nil {
		return "", nil, err
	}

	if len(form.Fields) == 0 {
		return "", nil, errors.New("cannot publish: add at least one field")
	}

	// ensure fields valid (published mode)
	dtoFields := make([]models.FormFieldDTO, 0, len(form.Fields))
	for _, f := range form.Fields {
		dtoFields = append(dtoFields, models.FormFieldDTO{
			ID:       f.ID,
			Key:      f.Key,
			Label:    f.Label,
			Type:     string(f.Type),
			Required: f.Required,
			Order:    f.Order,
			Options:  decodeOptionsToDTO(f.Options),
		})
	}
	if _, err := buildAndValidateFields(form.ID, dtoFields, false); err != nil {
		return "", nil, err
	}

	settings, _ := decodeSettings(form.Settings)
	if settings.ExpiresAt != nil && strings.TrimSpace(*settings.ExpiresAt) != "" {
		t, parseErr := parseFlexibleTime(*settings.ExpiresAt)
		if parseErr != nil {
			return "", nil, errors.New("form expiresAt is invalid on server")
		}
		if !t.After(time.Now()) {
			return "", nil, errors.New("expiresAt must be in the future to publish")
		}
	}

	slug := ""
	if form.Slug != nil {
		slug = strings.TrimSpace(*form.Slug)
	}
	if slug == "" {
		base := slugify(form.Title)
		if base == "" {
			base = "form"
		}

		slug = base
		i := 2
		for {
			exists, err := s.repo.SlugExists(slug)
			if err != nil {
				return "", nil, err
			}
			if !exists {
				break
			}
			slug = fmt.Sprintf("%s-%d", base, i)
			i++
		}
	}

	prevStatus := form.Status
	now := time.Now().UTC()
	form.IsPublished = true
	form.Status = models.FormStatusPublished
	form.Slug = &slug
	if form.PublishedAt == nil || prevStatus != models.FormStatusPublished {
		form.PublishedAt = &now
	}

	if err := s.repo.Update(form); err != nil {
		return "", nil, err
	}

	publicURL := s.buildPublicURL(slug)
	if form.EventID != nil && s.eventRepo != nil && publicURL != nil {
		ev, evErr := s.eventRepo.GetByID(*form.EventID)
		if evErr != nil {
			return "", nil, evErr
		}
		ev.RegisterLink = publicURL
		if err := s.eventRepo.Update(ev); err != nil {
			return "", nil, err
		}
	}

	return slug, publicURL, nil
}

func (s *formService) GetOrCreateReportLink(formID string) (*models.FormReportLinkPayload, error) {
	form, err := s.repo.GetByID(formID)
	if err != nil {
		return nil, err
	}

	token, err := s.ensureReportAccessToken(form)
	if err != nil {
		return nil, err
	}

	return s.buildReportLinkPayload(form, token), nil
}

func (s *formService) GetPublic(slug string) (*models.PublicFormPayload, error) {
	form, err := s.repo.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	form.Fields = applyImplicitFieldVisibilityDefaults(form.Fields)

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

	cleanValues, err := validateSubmission(fields, req.Values)
	if err != nil {
		return err
	}
	if len(cleanValues) == 0 {
		return errors.New("at least one field is required")
	}

	valuesJSON, err := json.Marshal(cleanValues)
	if err != nil {
		return errors.New("failed to store submission")
	}

	// Extract common fields into columns for analytics
	name, email, phone, addr := extractCommonFields(fields, cleanValues)

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

	responseEmailEnabled := true
	if settings != nil && settings.ResponseEmailEnabled != nil {
		responseEmailEnabled = *settings.ResponseEmailEnabled
	}

	if regCode != nil && email != nil && s.sender != nil && !responseEmailEnabled {
		s.sendRegistrationCodeEmail(form, *email, name, *regCode)
	}
	if email != nil {
		s.sendResponseEmail(form, settings, cleanValues, name, *email, regCode, sub.ID)
	}
	if err := s.syncSubmissionTarget(form, settings, cleanValues); err != nil {
		log.Printf("⚠️ submission target sync failed: %v", err)
	}

	return nil
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

func (s *formService) Stats(start, end *time.Time) (*models.FormStatsResponse, error) {
	total, err := s.repo.CountSubmissionsFiltered("", start, end)
	if err != nil {
		return nil, err
	}

	perForm, err := s.repo.CountSubmissionsByForm(start, end)
	if err != nil {
		return nil, err
	}

	recent, err := s.repo.ListRecentSubmissions(10, start, end)
	if err != nil {
		return nil, err
	}

	return &models.FormStatsResponse{
		TotalSubmissions: total,
		PerForm:          perForm,
		Recent:           recent,
	}, nil
}

func (s *formService) StatsByForm(formID string, start, end *time.Time) ([]models.FormSubmissionDailyCount, error) {
	return s.repo.CountSubmissionsByDay(formID, start, end)
}

func (s *formService) CleanupExpiredForms(now time.Time) (int64, error) {
	count, err := s.repo.DeleteExpired(now)
	if err != nil {
		if isMissingRelationErr(err) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

/* =========================
   Helpers: settings/options
========================= */

func isMissingRelationErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return false
}

func isUniqueViolationErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func parseFlexibleTime(value string) (time.Time, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return time.Time{}, errors.New("empty time")
	}
	for _, layout := range flexibleTimeLayouts {
		if t, err := time.Parse(layout, val); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid time")
}

func normalizeFlexibleTime(value string) (string, error) {
	t, err := parseFlexibleTime(value)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.RFC3339), nil
}

func normalizeAbsoluteTemplateURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	candidate := trimmed
	if strings.HasPrefix(candidate, "//") {
		candidate = "https:" + candidate
	}
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}

	parsed, err := url.Parse(candidate)
	if err != nil || parsed == nil {
		return "", fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("unsupported URL scheme")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("missing URL host")
	}
	return parsed.String(), nil
}

func normalizeFormContentSections(sections *[]models.FormContentSectionDTO) error {
	if sections == nil {
		return nil
	}

	cleanSections := make([]models.FormContentSectionDTO, 0, len(*sections))
	seenIDs := map[string]bool{}

	for i, section := range *sections {
		title := strings.TrimSpace(section.Title)
		if title == "" {
			return fmt.Errorf("sections[%d].title is required", i)
		}
		if utf8.RuneCountInString(title) > 160 {
			return fmt.Errorf("sections[%d].title too long", i)
		}
		section.Title = title

		if section.Subtitle != nil {
			subtitle := strings.TrimSpace(*section.Subtitle)
			if subtitle == "" {
				section.Subtitle = nil
			} else {
				if utf8.RuneCountInString(subtitle) > 260 {
					return fmt.Errorf("sections[%d].subtitle too long", i)
				}
				section.Subtitle = &subtitle
			}
		}

		sectionID := slugify(title)
		if section.ID != nil {
			if candidate := slugify(*section.ID); candidate != "" {
				sectionID = candidate
			}
		}
		if sectionID == "" {
			sectionID = fmt.Sprintf("section-%d", i+1)
		}
		if seenIDs[sectionID] {
			return fmt.Errorf("sections[%d].id duplicates another section", i)
		}
		seenIDs[sectionID] = true
		section.ID = &sectionID

		if section.Layout != nil {
			layout := strings.ToLower(strings.TrimSpace(*section.Layout))
			switch layout {
			case "":
				section.Layout = nil
			case "grid", "stack", "timeline", "split":
				section.Layout = &layout
			default:
				return fmt.Errorf("sections[%d].layout must be grid, stack, timeline, or split", i)
			}
		}

		cleanItems := make([]models.FormContentSectionItemDTO, 0, len(section.Items))
		for j, item := range section.Items {
			itemTitle := strings.TrimSpace(item.Title)
			if itemTitle == "" {
				return fmt.Errorf("sections[%d].items[%d].title is required", i, j)
			}
			if utf8.RuneCountInString(itemTitle) > 140 {
				return fmt.Errorf("sections[%d].items[%d].title too long", i, j)
			}
			item.Title = itemTitle

			normalizeOptionalText := func(value **string, max int, fieldName string) error {
				if *value == nil {
					return nil
				}
				trimmed := strings.TrimSpace(**value)
				if trimmed == "" {
					*value = nil
					return nil
				}
				if utf8.RuneCountInString(trimmed) > max {
					return fmt.Errorf("sections[%d].items[%d].%s too long", i, j, fieldName)
				}
				*value = &trimmed
				return nil
			}

			if err := normalizeOptionalText(&item.Body, 320, "body"); err != nil {
				return err
			}
			if err := normalizeOptionalText(&item.Eyebrow, 80, "eyebrow"); err != nil {
				return err
			}
			if err := normalizeOptionalText(&item.Icon, 40, "icon"); err != nil {
				return err
			}
			if err := normalizeOptionalText(&item.LinkText, 80, "linkText"); err != nil {
				return err
			}
			if item.LinkURL != nil {
				raw := strings.TrimSpace(*item.LinkURL)
				if raw == "" {
					item.LinkURL = nil
				} else {
					normalized, err := normalizeAbsoluteTemplateURL(raw)
					if err != nil {
						return fmt.Errorf("sections[%d].items[%d].linkUrl is invalid", i, j)
					}
					item.LinkURL = &normalized
				}
			}

			cleanItems = append(cleanItems, item)
		}
		section.Items = cleanItems
		cleanSections = append(cleanSections, section)
	}

	*sections = cleanSections
	return nil
}

func encodeSettings(s *models.FormSettingsDTO) (datatypes.JSON, error) {
	if s == nil {
		return datatypes.JSON([]byte("null")), nil
	}
	if s.Capacity != nil && *s.Capacity < 0 {
		return nil, errors.New("capacity cannot be negative")
	}
	if s.ClosesAt != nil && strings.TrimSpace(*s.ClosesAt) != "" {
		normalized, err := normalizeFlexibleTime(*s.ClosesAt)
		if err != nil {
			return nil, errors.New("closesAt must be RFC3339 or YYYY-MM-DDTHH:MM")
		}
		s.ClosesAt = &normalized
	}
	if s.ExpiresAt != nil && strings.TrimSpace(*s.ExpiresAt) != "" {
		normalized, err := normalizeFlexibleTime(*s.ExpiresAt)
		if err != nil {
			return nil, errors.New("expiresAt must be RFC3339 or YYYY-MM-DDTHH:MM")
		}
		s.ExpiresAt = &normalized
	}

	normalizeText := func(name string, val *string, max int) (*string, error) {
		if val == nil {
			return nil, nil
		}
		v := strings.TrimSpace(*val)
		if v == "" {
			return nil, nil
		}
		if len(v) > max {
			return nil, fmt.Errorf("%s too long (max %d chars)", name, max)
		}
		return &v, nil
	}

	normalizeColor := func(name string, val *string) (*string, error) {
		if val == nil {
			return nil, nil
		}
		v := strings.TrimSpace(*val)
		if v == "" {
			return nil, nil
		}
		if !hexColorRe.MatchString(v) {
			return nil, fmt.Errorf("%s must be hex like #RRGGBB", name)
		}
		v = strings.ToLower(v)
		return &v, nil
	}

	// Root-level extended fields (kept for frontend compatibility)
	var err error
	if s.SuccessMessage, err = normalizeText("successMessage", s.SuccessMessage, 400); err != nil {
		return nil, err
	}
	if s.ResponseEmailSubject, err = normalizeText("responseEmailSubject", s.ResponseEmailSubject, 160); err != nil {
		return nil, err
	}
	if s.CampaignEmailSubject, err = normalizeText("campaignEmailSubject", s.CampaignEmailSubject, 160); err != nil {
		return nil, err
	}
	if s.ResponseEmailTemplateID != nil {
		id := strings.TrimSpace(*s.ResponseEmailTemplateID)
		if id == "" {
			s.ResponseEmailTemplateID = nil
		} else if len(id) > 120 {
			return nil, fmt.Errorf("responseEmailTemplateId too long")
		} else {
			s.ResponseEmailTemplateID = &id
		}
	}
	if s.CampaignEmailTemplateID != nil {
		id := strings.TrimSpace(*s.CampaignEmailTemplateID)
		if id == "" {
			s.CampaignEmailTemplateID = nil
		} else if len(id) > 120 {
			return nil, fmt.Errorf("campaignEmailTemplateId too long")
		} else {
			s.CampaignEmailTemplateID = &id
		}
	}
	if s.ResponseEmailTemplateURL != nil {
		raw := strings.TrimSpace(*s.ResponseEmailTemplateURL)
		if raw == "" {
			s.ResponseEmailTemplateURL = nil
		} else {
			normalized, parseErr := normalizeAbsoluteTemplateURL(raw)
			if parseErr != nil {
				return nil, fmt.Errorf("responseEmailTemplateUrl is invalid")
			} else {
				s.ResponseEmailTemplateURL = &normalized
			}
		}
	}
	if s.CampaignEmailTemplateURL != nil {
		raw := strings.TrimSpace(*s.CampaignEmailTemplateURL)
		if raw == "" {
			s.CampaignEmailTemplateURL = nil
		} else {
			normalized, parseErr := normalizeAbsoluteTemplateURL(raw)
			if parseErr != nil {
				return nil, fmt.Errorf("campaignEmailTemplateUrl is invalid")
			} else {
				s.CampaignEmailTemplateURL = &normalized
			}
		}
	}
	if s.ResponseEmailTemplateKey != nil {
		key := strings.Trim(strings.TrimSpace(*s.ResponseEmailTemplateKey), "/")
		if key == "" {
			s.ResponseEmailTemplateKey = nil
		} else {
			if strings.HasPrefix(strings.ToLower(key), "http://") || strings.HasPrefix(strings.ToLower(key), "https://") {
				// Allow legacy clients that stored a full template image URL in templateKey.
				normalized, parseErr := normalizeAbsoluteTemplateURL(key)
				if parseErr == nil && s.ResponseEmailTemplateURL == nil {
					u := normalized
					s.ResponseEmailTemplateURL = &u
				}
				s.ResponseEmailTemplateKey = nil
			} else {
				if strings.Contains(key, "..") || !templateKeyRe.MatchString(key) {
					return nil, fmt.Errorf("responseEmailTemplateKey contains invalid characters")
				}
				s.ResponseEmailTemplateKey = &key
			}
		}
	}
	if s.CampaignEmailTemplateKey != nil {
		key := strings.Trim(strings.TrimSpace(*s.CampaignEmailTemplateKey), "/")
		if key == "" {
			s.CampaignEmailTemplateKey = nil
		} else {
			if strings.HasPrefix(strings.ToLower(key), "http://") || strings.HasPrefix(strings.ToLower(key), "https://") {
				normalized, parseErr := normalizeAbsoluteTemplateURL(key)
				if parseErr == nil && s.CampaignEmailTemplateURL == nil {
					u := normalized
					s.CampaignEmailTemplateURL = &u
				}
				s.CampaignEmailTemplateKey = nil
			} else {
				if strings.Contains(key, "..") || !templateKeyRe.MatchString(key) {
					return nil, fmt.Errorf("campaignEmailTemplateKey contains invalid characters")
				}
				s.CampaignEmailTemplateKey = &key
			}
		}
	}
	if s.SubmissionTarget != nil {
		target := strings.ToLower(strings.TrimSpace(*s.SubmissionTarget))
		switch target {
		case "", "none":
			s.SubmissionTarget = nil
		case "workforce", "workforce_new", "workforce_serving", "member", "leadership", "testimonial":
			s.SubmissionTarget = &target
		default:
			return nil, fmt.Errorf("submissionTarget must be workforce, workforce_new, workforce_serving, member, leadership, or testimonial")
		}
	}
	if s.SubmissionDepartment, err = normalizeText("submissionDepartment", s.SubmissionDepartment, 120); err != nil {
		return nil, err
	}
	if s.FormType != nil {
		formType := strings.ToLower(strings.TrimSpace(*s.FormType))
		switch formType {
		case "":
			s.FormType = nil
		case "registration", "event", "membership", "workforce", "leadership", "application", "contact", "general":
			s.FormType = &formType
		default:
			return nil, fmt.Errorf("formType must be one of: registration, event, membership, workforce, leadership, application, contact, general")
		}
	}
	if s.IntroTitle, err = normalizeText("introTitle", s.IntroTitle, 200); err != nil {
		return nil, err
	}
	if s.IntroSubtitle, err = normalizeText("introSubtitle", s.IntroSubtitle, 260); err != nil {
		return nil, err
	}
	if s.FormHeaderNote, err = normalizeText("formHeaderNote", s.FormHeaderNote, 400); err != nil {
		return nil, err
	}
	if s.SubmitButtonText, err = normalizeText("submitButtonText", s.SubmitButtonText, 120); err != nil {
		return nil, err
	}
	if s.FooterText, err = normalizeText("footerText", s.FooterText, 300); err != nil {
		return nil, err
	}

	if s.FooterBg, err = normalizeColor("footerBg", s.FooterBg); err != nil {
		return nil, err
	}
	if s.FooterTextColor, err = normalizeColor("footerTextColor", s.FooterTextColor); err != nil {
		return nil, err
	}
	if s.SubmitButtonBg, err = normalizeColor("submitButtonBg", s.SubmitButtonBg); err != nil {
		return nil, err
	}
	if s.SubmitButtonTextColor, err = normalizeColor("submitButtonTextColor", s.SubmitButtonTextColor); err != nil {
		return nil, err
	}

	if s.SubmitButtonIcon != nil {
		icon := strings.ToLower(strings.TrimSpace(*s.SubmitButtonIcon))
		switch icon {
		case "", "check", "send", "calendar", "cursor", "none":
			if icon == "" {
				s.SubmitButtonIcon = nil
			} else {
				s.SubmitButtonIcon = &icon
			}
		default:
			return nil, fmt.Errorf("submitButtonIcon must be one of: check, send, calendar, cursor, none")
		}
	}

	if s.LayoutMode != nil {
		lm := strings.ToLower(strings.TrimSpace(*s.LayoutMode))
		if lm == "" {
			s.LayoutMode = nil
		} else {
			if lm == "stacked" {
				lm = "stack"
			}
			if lm != "split" && lm != "stack" {
				return nil, fmt.Errorf("layoutMode must be split or stack")
			}
			s.LayoutMode = &lm
		}
	}

	if s.DateFormat != nil {
		df := strings.TrimSpace(*s.DateFormat)
		switch df {
		case "yyyy-mm-dd", "mm/dd/yyyy", "dd/mm/yyyy", "dd/mm":
			s.DateFormat = &df
		case "":
			s.DateFormat = nil
		default:
			return nil, fmt.Errorf("dateFormat invalid")
		}
	}

	if s.IntroBullets != nil {
		clean := make([]string, 0, len(*s.IntroBullets))
		for _, b := range *s.IntroBullets {
			b = strings.TrimSpace(b)
			if b != "" && utf8.RuneCountInString(b) <= 200 {
				clean = append(clean, b)
			}
		}
		s.IntroBullets = &clean
	}
	if s.IntroBulletSubtexts != nil {
		clean := make([]string, 0, len(*s.IntroBulletSubtexts))
		for _, b := range *s.IntroBulletSubtexts {
			b = strings.TrimSpace(b)
			if b != "" && utf8.RuneCountInString(b) <= 200 {
				clean = append(clean, b)
			}
		}
		s.IntroBulletSubtexts = &clean
	}
	if err := normalizeFormContentSections(s.Sections); err != nil {
		return nil, err
	}

	// Normalize and validate optional design settings
	if s.Design != nil {
		d := s.Design

		if layout := d.Layout; layout != nil {
			lv := strings.ToLower(strings.TrimSpace(*layout))
			if lv == "" {
				d.Layout = nil
			} else if lv != "split" && lv != "stacked" && lv != "inline" && lv != "stack" {
				return nil, fmt.Errorf("design.layout must be one of: split, stacked, stack, inline")
			} else {
				d.Layout = &lv
			}
		}

		var err error
		if d.HeroTitle, err = normalizeText("design.heroTitle", d.HeroTitle, 160); err != nil {
			return nil, err
		}
		if d.HeroSubtitle, err = normalizeText("design.heroSubtitle", d.HeroSubtitle, 260); err != nil {
			return nil, err
		}
		if d.CoverImageURL, err = normalizeText("design.coverImageUrl", d.CoverImageURL, 500); err != nil {
			return nil, err
		}
		if d.CTAButtonLabel, err = normalizeText("design.ctaButtonLabel", d.CTAButtonLabel, 80); err != nil {
			return nil, err
		}
		if d.PrivacyCopy, err = normalizeText("design.privacyCopy", d.PrivacyCopy, 600); err != nil {
			return nil, err
		}
		if d.FooterNote, err = normalizeText("design.footerNote", d.FooterNote, 300); err != nil {
			return nil, err
		}
		if d.FooterText, err = normalizeText("design.footerText", d.FooterText, 300); err != nil {
			return nil, err
		}
		if d.FormHeaderNote, err = normalizeText("design.formHeaderNote", d.FormHeaderNote, 400); err != nil {
			return nil, err
		}
		if d.SubmitButtonText, err = normalizeText("design.submitButtonText", d.SubmitButtonText, 120); err != nil {
			return nil, err
		}
		if d.IntroTitle, err = normalizeText("design.introTitle", d.IntroTitle, 200); err != nil {
			return nil, err
		}
		if d.IntroSubtitle, err = normalizeText("design.introSubtitle", d.IntroSubtitle, 260); err != nil {
			return nil, err
		}
		if d.PrimaryColor, err = normalizeColor("design.primaryColor", d.PrimaryColor); err != nil {
			return nil, err
		}
		if d.BackgroundColor, err = normalizeColor("design.backgroundColor", d.BackgroundColor); err != nil {
			return nil, err
		}
		if d.AccentColor, err = normalizeColor("design.accentColor", d.AccentColor); err != nil {
			return nil, err
		}
		if d.FooterBg, err = normalizeColor("design.footerBg", d.FooterBg); err != nil {
			return nil, err
		}
		if d.FooterTextColor, err = normalizeColor("design.footerTextColor", d.FooterTextColor); err != nil {
			return nil, err
		}
		if d.SubmitButtonBg, err = normalizeColor("design.submitButtonBg", d.SubmitButtonBg); err != nil {
			return nil, err
		}
		if d.SubmitButtonTextColor, err = normalizeColor("design.submitButtonTextColor", d.SubmitButtonTextColor); err != nil {
			return nil, err
		}

		if d.SubmitButtonIcon != nil {
			icon := strings.ToLower(strings.TrimSpace(*d.SubmitButtonIcon))
			switch icon {
			case "", "check", "send", "calendar", "cursor", "none":
				if icon == "" {
					d.SubmitButtonIcon = nil
				} else {
					d.SubmitButtonIcon = &icon
				}
			default:
				return nil, fmt.Errorf("design.submitButtonIcon must be one of: check, send, calendar, cursor, none")
			}
		}

		if d.LayoutMode != nil {
			lm := strings.ToLower(strings.TrimSpace(*d.LayoutMode))
			if lm == "" {
				d.LayoutMode = nil
			} else {
				if lm == "stacked" {
					lm = "stack"
				}
				if lm != "split" && lm != "stack" {
					return nil, fmt.Errorf("design.layoutMode must be split or stack")
				}
				d.LayoutMode = &lm
			}
		}

		if d.DateFormat != nil {
			df := strings.TrimSpace(*d.DateFormat)
			switch df {
			case "yyyy-mm-dd", "mm/dd/yyyy", "dd/mm/yyyy", "dd/mm":
				d.DateFormat = &df
			case "":
				d.DateFormat = nil
			default:
				return nil, fmt.Errorf("design.dateFormat invalid")
			}
		}

		if d.IntroBullets != nil {
			clean := make([]string, 0, len(*d.IntroBullets))
			for _, b := range *d.IntroBullets {
				b = strings.TrimSpace(b)
				if b != "" && utf8.RuneCountInString(b) <= 200 {
					clean = append(clean, b)
				}
			}
			d.IntroBullets = &clean
		}
		if d.IntroBulletSubtext != nil {
			clean := make([]string, 0, len(*d.IntroBulletSubtext))
			for _, b := range *d.IntroBulletSubtext {
				b = strings.TrimSpace(b)
				if b != "" && utf8.RuneCountInString(b) <= 200 {
					clean = append(clean, b)
				}
			}
			d.IntroBulletSubtext = &clean
		}
	}

	b, err := json.Marshal(s)
	if err != nil {
		return nil, errors.New("invalid settings")
	}
	return datatypes.JSON(b), nil
}

func decodeSettings(j datatypes.JSON) (*models.FormSettingsDTO, error) {
	if len(j) == 0 || string(j) == "null" {
		return &models.FormSettingsDTO{}, nil
	}
	var s models.FormSettingsDTO
	if err := json.Unmarshal(j, &s); err != nil {
		return &models.FormSettingsDTO{}, err
	}
	return &s, nil
}

func mergeFormSettings(base *models.FormSettingsDTO, incoming *models.FormSettingsDTO) (*models.FormSettingsDTO, error) {
	if base == nil {
		base = &models.FormSettingsDTO{}
	}
	if incoming == nil {
		return base, nil
	}

	baseMap := map[string]any{}
	if raw, err := json.Marshal(base); err == nil {
		_ = json.Unmarshal(raw, &baseMap)
	}

	incMap := map[string]any{}
	if raw, err := json.Marshal(incoming); err == nil {
		_ = json.Unmarshal(raw, &incMap)
	}

	mergeStringAnyMaps(baseMap, incMap)

	raw, err := json.Marshal(baseMap)
	if err != nil {
		return nil, err
	}
	var merged models.FormSettingsDTO
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

func mergeStringAnyMaps(dst map[string]any, src map[string]any) {
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			if dstMap, ok := dst[k].(map[string]any); ok {
				mergeStringAnyMaps(dstMap, vMap)
				dst[k] = dstMap
				continue
			}
		}
		dst[k] = v
	}
}

func decodeOptionsToDTO(j datatypes.JSON) []models.FormFieldOptionDTO {
	if len(j) == 0 || string(j) == "null" {
		return nil
	}
	var opts []models.FormFieldOptionDTO
	_ = json.Unmarshal(j, &opts)
	return opts
}

/* =========================
   Helpers: slugify
========================= */

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugInvalidRe.ReplaceAllString(s, "")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	return s
}

/* =========================
   Field validation/build
========================= */

func buildAndValidateFields(formID string, fields []models.FormFieldDTO, draftOK bool) ([]models.FormField, error) {
	if !draftOK && len(fields) == 0 {
		return nil, errors.New("add at least one field")
	}

	type normalizedField struct {
		dto   models.FormFieldDTO
		key   string
		label string
		typ   string
	}

	seenKey := map[string]bool{}
	normalized := make([]normalizedField, 0, len(fields))
	out := make([]models.FormField, 0, len(fields))

	for i, f := range fields {
		label := strings.TrimSpace(f.Label)
		typ := normalizeFieldType(f.Type)
		key := strings.TrimSpace(f.Key)
		if key == "" && label != "" {
			key = slugify(label)
		}
		key = strings.ToLower(strings.TrimSpace(key))

		if key == "" {
			return nil, fmt.Errorf("field[%d]: key is required", i)
		}
		if label == "" {
			return nil, fmt.Errorf("field[%d]: label is required", i)
		}
		if seenKey[key] {
			return nil, fmt.Errorf("field[%d]: duplicate key '%s'", i, key)
		}
		seenKey[key] = true

		if !isValidFieldType(typ) {
			return nil, fmt.Errorf("field[%d]: invalid type '%s'", i, typ)
		}

		normalized = append(normalized, normalizedField{
			dto:   f,
			key:   key,
			label: label,
			typ:   typ,
		})
	}

	for i, nf := range normalized {
		f := nf.dto

		var optionsJSON datatypes.JSON
		if nf.typ == string(models.FieldSelect) || nf.typ == string(models.FieldRadio) {
			if len(f.Options) == 0 {
				return nil, fmt.Errorf("field[%d]: options required for type '%s'", i, nf.typ)
			}
			seenVal := map[string]bool{}
			for oi, opt := range f.Options {
				if strings.TrimSpace(opt.Label) == "" || strings.TrimSpace(opt.Value) == "" {
					return nil, fmt.Errorf("field[%d].options[%d]: label and value required", i, oi)
				}
				if seenVal[opt.Value] {
					return nil, fmt.Errorf("field[%d].options[%d]: duplicate value '%s'", i, oi, opt.Value)
				}
				seenVal[opt.Value] = true
			}
			b, _ := json.Marshal(f.Options)
			optionsJSON = datatypes.JSON(b)
		} else if nf.typ == string(models.FieldCheckbox) && len(f.Options) > 0 {
			seenVal := map[string]bool{}
			for oi, opt := range f.Options {
				if strings.TrimSpace(opt.Label) == "" || strings.TrimSpace(opt.Value) == "" {
					return nil, fmt.Errorf("field[%d].options[%d]: label and value required", i, oi)
				}
				if seenVal[opt.Value] {
					return nil, fmt.Errorf("field[%d].options[%d]: duplicate value '%s'", i, oi, opt.Value)
				}
				seenVal[opt.Value] = true
			}
			b, _ := json.Marshal(f.Options)
			optionsJSON = datatypes.JSON(b)
		} else {
			optionsJSON = datatypes.JSON([]byte("null"))
		}

		validationJSON, err := normalizeValidationRules(nf.typ, f.Validation)
		if err != nil {
			return nil, fmt.Errorf("field[%d]: %w", i, err)
		}

		visibilityJSON, err := normalizeVisibility(f.Visibility, seenKey, nf.key)
		if err != nil {
			return nil, fmt.Errorf("field[%d]: %w", i, err)
		}

		out = append(out, models.FormField{
			FormID:     formID,
			Key:        nf.key,
			Label:      nf.label,
			Type:       models.FormFieldType(nf.typ),
			Required:   f.Required,
			Options:    optionsJSON,
			Validation: validationJSON,
			Visibility: visibilityJSON,
			Order:      f.Order,
		})
	}

	return out, nil
}

func isValidFieldType(t string) bool {
	switch t {
	case string(models.FieldText),
		string(models.FieldEmail),
		string(models.FieldTel),
		string(models.FieldTextarea),
		string(models.FieldSelect),
		string(models.FieldCheckbox),
		string(models.FieldRadio),
		string(models.FieldNumber),
		string(models.FieldDate):
		return true
	default:
		return false
	}
}

func normalizeFieldType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "phone":
		return string(models.FieldTel)
	case "dropdown", "select_one", "single_select", "selectone":
		return string(models.FieldSelect)
	case "paragraph", "long_text", "longtext", "multi_line", "multiline":
		return string(models.FieldTextarea)
	case "short_text", "shorttext", "single_line", "singleline":
		return string(models.FieldText)
	default:
		return t
	}
}

// normalizeValidationRules ensures provided validation config is coherent for the field type
// and returns a compact JSON blob (or null) to store.
func normalizeValidationRules(fieldType string, v *models.FormFieldValidation) (datatypes.JSON, error) {
	if v == nil {
		return datatypes.JSON([]byte("null")), nil
	}

	rules := *v // shallow copy so we can sanitize

	// Basic numeric checks
	if rules.MinLength != nil && *rules.MinLength < 0 {
		return nil, fmt.Errorf("minLength cannot be negative")
	}
	if rules.MaxLength != nil && *rules.MaxLength < 0 {
		return nil, fmt.Errorf("maxLength cannot be negative")
	}
	if rules.MinLength != nil && rules.MaxLength != nil && *rules.MaxLength < *rules.MinLength {
		return nil, fmt.Errorf("maxLength cannot be less than minLength")
	}
	if rules.MaxWords != nil && *rules.MaxWords <= 0 {
		return nil, fmt.Errorf("maxWords must be greater than 0")
	}
	if rules.Min != nil && rules.Max != nil && *rules.Max < *rules.Min {
		return nil, fmt.Errorf("max cannot be less than min")
	}

	// Allow numeric bounds only on number fields
	if fieldType != string(models.FieldNumber) && (rules.Min != nil || rules.Max != nil) {
		return nil, fmt.Errorf("min/max are only valid for number fields")
	}

	// Allow string-oriented rules only on string-input fields
	if !isStringFieldType(fieldType) {
		if rules.MinLength != nil || rules.MaxLength != nil || rules.MaxWords != nil || rules.Pattern != nil {
			return nil, fmt.Errorf("string validation rules are not applicable to this field type")
		}
	}

	if rules.Pattern != nil {
		p := strings.TrimSpace(*rules.Pattern)
		if p == "" {
			rules.Pattern = nil
		} else {
			if _, err := regexp.Compile(p); err != nil {
				return nil, fmt.Errorf("pattern is not a valid regex")
			}
			rules.Pattern = &p
		}
	}

	if !hasAnyValidationRule(&rules) {
		return datatypes.JSON([]byte("null")), nil
	}

	b, err := json.Marshal(rules)
	if err != nil {
		return nil, errors.New("invalid validation rules")
	}
	return datatypes.JSON(b), nil
}

func normalizeVisibility(v *models.FormFieldVisibility, keys map[string]bool, selfKey string) (datatypes.JSON, error) {
	if v == nil || len(v.Rules) == 0 {
		return datatypes.JSON([]byte("null")), nil
	}

	match := strings.ToLower(strings.TrimSpace(v.Match))
	if match == "" {
		match = "all"
	}
	if match != "all" && match != "any" {
		return nil, fmt.Errorf("visibility.match must be 'all' or 'any'")
	}
	v.Match = match

	for i := range v.Rules {
		r := v.Rules[i]
		key := strings.ToLower(strings.TrimSpace(r.FieldKey))
		if key == "" {
			return nil, fmt.Errorf("visibility.rules[%d].fieldKey is required", i)
		}
		if key == selfKey {
			return nil, fmt.Errorf("visibility.rules[%d].fieldKey cannot reference itself", i)
		}
		if keys != nil && !keys[key] {
			return nil, fmt.Errorf("visibility.rules[%d].fieldKey '%s' not found", i, key)
		}

		op := strings.ToLower(strings.TrimSpace(r.Operator))
		if op == "" {
			return nil, fmt.Errorf("visibility.rules[%d].operator is required", i)
		}
		switch op {
		case "equals", "not_equals":
			if r.Value == nil {
				return nil, fmt.Errorf("visibility.rules[%d].value is required", i)
			}
		case "in", "not_in":
			if len(r.Values) == 0 {
				return nil, fmt.Errorf("visibility.rules[%d].values is required", i)
			}
		default:
			return nil, fmt.Errorf("visibility.rules[%d].operator must be one of: equals, not_equals, in, not_in", i)
		}

		r.FieldKey = key
		r.Operator = op
		v.Rules[i] = r
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, errors.New("invalid visibility rules")
	}
	return datatypes.JSON(b), nil
}

func isStringFieldType(t string) bool {
	switch t {
	case string(models.FieldText),
		string(models.FieldEmail),
		string(models.FieldTel),
		string(models.FieldTextarea),
		string(models.FieldSelect),
		string(models.FieldRadio),
		string(models.FieldDate):
		return true
	default:
		return false
	}
}

func hasAnyValidationRule(v *models.FormFieldValidation) bool {
	return v != nil &&
		(v.MinLength != nil ||
			v.MaxLength != nil ||
			v.MaxWords != nil ||
			v.Pattern != nil ||
			v.Min != nil ||
			v.Max != nil)
}

func decodeVisibility(j datatypes.JSON) *models.FormFieldVisibility {
	if len(j) == 0 || string(j) == "null" {
		return nil
	}
	var v models.FormFieldVisibility
	if err := json.Unmarshal(j, &v); err != nil {
		return nil
	}
	if len(v.Rules) == 0 {
		return nil
	}
	if strings.TrimSpace(v.Match) == "" {
		v.Match = "all"
	}
	return &v
}

func normalizeVisibilityToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isAffirmativeVisibilityOption(value string) bool {
	switch normalizeVisibilityToken(value) {
	case "yes", "true", "1":
		return true
	default:
		return false
	}
}

func isNegativeVisibilityOption(value string) bool {
	switch normalizeVisibilityToken(value) {
	case "no", "false", "0":
		return true
	default:
		return false
	}
}

func looksLikeImplicitYesField(label string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), "if yes")
}

func findImplicitYesOptionValue(field models.FormField) (string, bool) {
	if field.Type != models.FieldRadio && field.Type != models.FieldSelect {
		return "", false
	}

	opts := decodeOptionsToDTO(field.Options)
	if len(opts) == 0 {
		return "", false
	}

	yesValue := ""
	hasNoOption := false
	for _, opt := range opts {
		label := strings.TrimSpace(opt.Label)
		value := strings.TrimSpace(opt.Value)

		if yesValue == "" && (isAffirmativeVisibilityOption(label) || isAffirmativeVisibilityOption(value)) {
			if value != "" {
				yesValue = value
			} else {
				yesValue = label
			}
		}
		if isNegativeVisibilityOption(label) || isNegativeVisibilityOption(value) {
			hasNoOption = true
		}
	}

	if yesValue == "" || !hasNoOption {
		return "", false
	}
	return yesValue, true
}

func applyImplicitFieldVisibilityDefaults(fields []models.FormField) []models.FormField {
	if len(fields) == 0 {
		return fields
	}

	enriched := append([]models.FormField(nil), fields...)
	sort.SliceStable(enriched, func(i, j int) bool {
		if enriched[i].Order == enriched[j].Order {
			return i < j
		}
		return enriched[i].Order < enriched[j].Order
	})

	for i := range enriched {
		if decodeVisibility(enriched[i].Visibility) != nil {
			continue
		}
		if !looksLikeImplicitYesField(enriched[i].Label) {
			continue
		}

		for prev := i - 1; prev >= 0; prev-- {
			fieldKey := strings.TrimSpace(enriched[prev].Key)
			yesValue, ok := findImplicitYesOptionValue(enriched[prev])
			if !ok || fieldKey == "" {
				continue
			}

			vis := models.FormFieldVisibility{
				Match: "all",
				Rules: []models.FormFieldCondition{
					{
						FieldKey: fieldKey,
						Operator: "equals",
						Value:    yesValue,
					},
				},
			}
			if raw, err := json.Marshal(vis); err == nil {
				enriched[i].Visibility = datatypes.JSON(raw)
			}
			break
		}
	}

	return enriched
}

func isFieldVisible(f models.FormField, values map[string]any) bool {
	vis := decodeVisibility(f.Visibility)
	if vis == nil || len(vis.Rules) == 0 {
		return true
	}

	match := strings.ToLower(strings.TrimSpace(vis.Match))
	if match != "any" && match != "all" {
		match = "all"
	}

	if match == "any" {
		for _, r := range vis.Rules {
			if evaluateVisibilityRule(r, values) {
				return true
			}
		}
		return false
	}

	for _, r := range vis.Rules {
		if !evaluateVisibilityRule(r, values) {
			return false
		}
	}
	return true
}

func evaluateVisibilityRule(r models.FormFieldCondition, values map[string]any) bool {
	key := strings.TrimSpace(r.FieldKey)
	if key == "" {
		return false
	}
	val, ok := values[key]
	if !ok || val == nil {
		return false
	}

	op := strings.ToLower(strings.TrimSpace(r.Operator))
	switch op {
	case "equals":
		return valueEquals(val, r.Value)
	case "not_equals":
		return !valueEquals(val, r.Value)
	case "in":
		return valueIn(val, r.Values)
	case "not_in":
		return !valueIn(val, r.Values)
	default:
		return false
	}
}

func valueEquals(a any, b any) bool {
	if a == nil || b == nil {
		return false
	}

	switch av := a.(type) {
	case bool:
		if bv, ok := b.(bool); ok {
			return av == bv
		}
	case string:
		if bs, ok := b.(string); ok {
			return strings.TrimSpace(av) == strings.TrimSpace(bs)
		}
	}

	if af, err := toFloat64(a); err == nil {
		if bf, err := toFloat64(b); err == nil {
			return af == bf
		}
	}

	return fmt.Sprint(a) == fmt.Sprint(b)
}

func valueIn(val any, list []any) bool {
	if len(list) == 0 {
		return false
	}
	for _, v := range list {
		if valueEquals(val, v) {
			return true
		}
	}
	return false
}

/* =========================
   Submission validation
========================= */

func validateSubmission(fields []models.FormField, values map[string]any) (map[string]any, error) {
	fields = applyImplicitFieldVisibilityDefaults(fields)

	fieldByKey := map[string]models.FormField{}
	for _, f := range fields {
		fieldByKey[f.Key] = f
	}

	for k := range values {
		if _, ok := fieldByKey[k]; !ok {
			return nil, fmt.Errorf("unknown field '%s'", k)
		}
	}

	// snapshot values for visibility evaluation
	snapshot := make(map[string]any, len(values))
	for k, v := range values {
		snapshot[k] = v
	}

	clean := make(map[string]any, len(values))

	for _, f := range fields {
		v, exists := snapshot[f.Key]

		if !isFieldVisible(f, snapshot) {
			continue
		}

		rules := decodeValidation(f.Validation)

		if !exists || v == nil {
			if f.Required {
				return nil, fmt.Errorf("field '%s' is required", f.Key)
			}
			continue
		}

		switch f.Type {
		case models.FieldCheckbox:
			opts := decodeOptionsToDTO(f.Options)
			if len(opts) > 0 {
				list, ok := toStringSlice(v)
				if !ok {
					return nil, fmt.Errorf("field '%s' must be a list of strings", f.Key)
				}

				allowed := map[string]bool{}
				for _, o := range opts {
					allowed[o.Value] = true
				}

				seen := map[string]bool{}
				cleaned := make([]string, 0, len(list))
				for _, raw := range list {
					s := strings.TrimSpace(raw)
					if s == "" {
						continue
					}
					if !allowed[s] {
						return nil, fmt.Errorf("field '%s' has invalid option", f.Key)
					}
					if !seen[s] {
						seen[s] = true
						cleaned = append(cleaned, s)
					}
				}

				if f.Required && len(cleaned) == 0 {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}

				clean[f.Key] = cleaned
				continue
			}

			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be boolean", f.Key)
			}
			if f.Required && !b {
				return nil, fmt.Errorf("field '%s' must be accepted", f.Key)
			}
			clean[f.Key] = b

		case models.FieldNumber:
			if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
				if f.Required {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}
				continue
			}

			num, err := toFloat64(v)
			if err != nil {
				return nil, fmt.Errorf("field '%s' must be a number", f.Key)
			}
			clean[f.Key] = num

			if rules != nil {
				if rules.Min != nil && num < *rules.Min {
					return nil, fmt.Errorf("field '%s' must be >= %g", f.Key, *rules.Min)
				}
				if rules.Max != nil && num > *rules.Max {
					return nil, fmt.Errorf("field '%s' must be <= %g", f.Key, *rules.Max)
				}
			}

		case models.FieldSelect, models.FieldRadio:
			sv, ok := valueToString(v)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be string", f.Key)
			}
			sv = strings.TrimSpace(sv)
			if sv == "" {
				if f.Required {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}
				continue
			}

			opts := decodeOptionsToDTO(f.Options)
			allowed := map[string]bool{}
			for _, o := range opts {
				allowed[o.Value] = true
			}
			if !allowed[sv] {
				return nil, fmt.Errorf("field '%s' has invalid option", f.Key)
			}
			clean[f.Key] = sv

			if err := applyStringRules(f.Key, sv, rules); err != nil {
				return nil, err
			}

		default: // text, textarea, email, tel, date
			sv, ok := valueToString(v)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be string", f.Key)
			}
			sv = strings.TrimSpace(sv)
			if sv == "" {
				if f.Required {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}
				continue
			}

			switch f.Type {
			case models.FieldEmail:
				if !emailRe.MatchString(sv) {
					return nil, fmt.Errorf("field '%s' must be a valid email", f.Key)
				}
			case models.FieldTel:
				if !phoneRe.MatchString(sv) {
					return nil, fmt.Errorf("field '%s' must be a valid phone number", f.Key)
				}
			case models.FieldDate:
				if _, err := time.Parse("2006-01-02", sv); err != nil {
					return nil, fmt.Errorf("field '%s' must be a valid date (YYYY-MM-DD)", f.Key)
				}
			}

			if err := applyStringRules(f.Key, sv, rules); err != nil {
				return nil, err
			}
			clean[f.Key] = sv
		}
	}

	return clean, nil
}

func applyStringRules(key, value string, rules *models.FormFieldValidation) error {
	if rules == nil {
		return nil
	}

	runeLen := utf8.RuneCountInString(value)
	if rules.MinLength != nil && runeLen < *rules.MinLength {
		return fmt.Errorf("field '%s' must be at least %d characters", key, *rules.MinLength)
	}
	if rules.MaxLength != nil && runeLen > *rules.MaxLength {
		return fmt.Errorf("field '%s' must be at most %d characters", key, *rules.MaxLength)
	}
	if rules.MaxWords != nil {
		if wc := countWords(value); wc > *rules.MaxWords {
			return fmt.Errorf("field '%s' must be at most %d words", key, *rules.MaxWords)
		}
	}
	if rules.Pattern != nil {
		re := regexp.MustCompile(*rules.Pattern)
		if !re.MatchString(value) {
			return fmt.Errorf("field '%s' does not match the required format", key)
		}
	}
	return nil
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

func firstToken(s string) string {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func valueToString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func toStringSlice(v any) ([]string, bool) {
	switch raw := v.(type) {
	case []string:
		return raw, true
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	case string:
		// allow single selection serialized as string
		if strings.TrimSpace(raw) == "" {
			return []string{}, true
		}
		return []string{raw}, true
	default:
		return nil, false
	}
}

func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case json.Number:
		return n.Float64()
	case string:
		s := strings.TrimSpace(n)
		return strconv.ParseFloat(s, 64)
	default:
		return 0, fmt.Errorf("not a number")
	}
}

func decodeValidation(j datatypes.JSON) *models.FormFieldValidation {
	if len(j) == 0 || string(j) == "null" {
		return nil
	}
	var v models.FormFieldValidation
	if err := json.Unmarshal(j, &v); err != nil {
		return nil
	}
	if !hasAnyValidationRule(&v) {
		return nil
	}
	return &v
}

// extractCommonFields pulls common contact fields from dynamic values map for analytics.
// It prefers known keys, then falls back to matching field types (email/tel) if present.
func extractCommonFields(fields []models.FormField, values map[string]any) (*string, *string, *string, *string) {
	lookup := func(keys ...string) *string {
		for _, k := range keys {
			if v, ok := values[k]; ok {
				if s, ok := v.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						return &s
					}
				}
			}
		}
		return nil
	}

	byType := func(t models.FormFieldType) *string {
		for _, f := range fields {
			if f.Type != t {
				continue
			}
			if v, ok := values[f.Key]; ok {
				if s, ok := v.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						return &s
					}
				}
			}
		}
		return nil
	}

	lookupByLabel := func(needles ...string) *string {
		for _, f := range fields {
			label := strings.ToLower(strings.TrimSpace(f.Label))
			if label == "" {
				continue
			}
			matched := false
			for _, needle := range needles {
				n := strings.ToLower(strings.TrimSpace(needle))
				if n == "" {
					continue
				}
				if strings.Contains(label, n) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			if raw, ok := values[f.Key]; ok {
				if s, ok := raw.(string); ok {
					v := strings.TrimSpace(s)
					if v != "" {
						return &v
					}
				}
			}
		}
		return nil
	}

	normalizeName := func(primary *string) *string {
		if primary != nil {
			v := strings.TrimSpace(*primary)
			if v != "" && !emailRe.MatchString(v) {
				return &v
			}
		}

		first := lookup("firstName", "first_name", "firstname", "first")
		last := lookup("lastName", "last_name", "lastname", "last")
		if first == nil {
			first = lookupByLabel("first name", "firstname")
		}
		if last == nil {
			last = lookupByLabel("last name", "lastname", "surname")
		}

		switch {
		case first != nil && last != nil:
			combined := strings.TrimSpace(*first + " " + *last)
			if combined != "" {
				return &combined
			}
		case first != nil:
			v := strings.TrimSpace(*first)
			if v != "" {
				return &v
			}
		case last != nil:
			v := strings.TrimSpace(*last)
			if v != "" {
				return &v
			}
		}

		// Legacy fallback: some older forms used "email" key for first-name text input.
		legacy := lookup("email")
		if legacy != nil {
			v := strings.TrimSpace(*legacy)
			if v != "" && !emailRe.MatchString(v) {
				return &v
			}
		}

		return nil
	}

	normalizeEmail := func(candidate *string) *string {
		if candidate != nil {
			v := strings.TrimSpace(*candidate)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		if typed := byType(models.FieldEmail); typed != nil {
			v := strings.TrimSpace(*typed)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		if labelled := lookupByLabel("email", "e-mail"); labelled != nil {
			v := strings.TrimSpace(*labelled)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		for _, raw := range values {
			s, ok := raw.(string)
			if !ok {
				continue
			}
			v := strings.TrimSpace(s)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		return nil
	}

	name := normalizeName(lookup("fullName", "name", "full_name"))
	email := normalizeEmail(lookup("email", "contactEmail"))
	phone := lookup("phone", "contactPhone", "contactNumber", "phoneNumber")
	if phone == nil {
		phone = byType(models.FieldTel)
	}
	addr := lookup("address", "contactAddress")

	return name, email, phone, addr
}

func (s *formService) buildRegistrationCode(form *models.Form) (string, error) {
	if form == nil || s.sequenceRepo == nil {
		return "", nil
	}
	title := strings.TrimSpace(form.Title)
	if form.EventID != nil && s.eventRepo != nil {
		if ev, err := s.eventRepo.GetByID(*form.EventID); err == nil && ev != nil {
			if strings.TrimSpace(ev.Title) != "" {
				title = strings.TrimSpace(ev.Title)
			}
		}
	}
	initials := buildInitials(title)
	if initials == "" {
		initials = "GEN"
	}
	year := time.Now().UTC().Format("06")
	prefix := fmt.Sprintf("WHC-%s-%s", initials, year)
	seq, err := s.sequenceRepo.Next(prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%06d", prefix, seq), nil
}

func buildInitials(value string) string {
	parts := regexp.MustCompile(`[A-Za-z0-9]+`).FindAllString(value, -1)
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		r := []rune(part)
		if len(r) == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(string(r[0])))
		if b.Len() >= 6 {
			break
		}
	}
	return b.String()
}

func (s *formService) sendRegistrationCodeEmail(form *models.Form, emailAddr string, name *string, code string) {
	if strings.TrimSpace(emailAddr) == "" || s.sender == nil {
		return
	}
	recipient := "there"
	if name != nil && strings.TrimSpace(*name) != "" {
		recipient = strings.TrimSpace(*name)
	}
	eventName := ""
	if form != nil {
		eventName = strings.TrimSpace(form.Title)
	}
	subject := "Your registration code"
	message := fmt.Sprintf("Your registration code for %s is %s.", eventName, code)
	body := email.RenderRegistrationCodeEmail(email.RegistrationCodeTemplateData{
		Branding:      s.branding,
		RecipientName: recipient,
		EventName:     eventName,
		Code:          code,
		Message:       message,
	})
	if err := s.sender.SendHTML(emailAddr, subject, body); err != nil {
		log.Printf("⚠️ failed to send registration code email to %s: %v", emailAddr, err)
	}
}

func (s *formService) sendResponseEmail(form *models.Form, settings *models.FormSettingsDTO, values map[string]any, name *string, emailAddr string, regCode *string, submissionID string) {
	if s.sender == nil {
		return
	}

	addr := strings.TrimSpace(emailAddr)
	if addr == "" {
		return
	}

	if settings != nil && settings.ResponseEmailEnabled != nil && !*settings.ResponseEmailEnabled {
		return
	}

	recipient := ""
	if name != nil {
		recipient = strings.TrimSpace(*name)
	}
	formID := ""
	if form != nil {
		formID = form.ID
	}

	event := &models.Event{}
	if form != nil && form.EventID != nil && s.eventRepo != nil {
		if ev, err := s.eventRepo.GetByID(*form.EventID); err == nil && ev != nil {
			event = ev
		}
	}

	formTitle := ""
	if form != nil {
		formTitle = strings.TrimSpace(form.Title)
	}
	if formTitle == "" {
		formTitle = strings.TrimSpace(event.Title)
	}

	subject := "Registration received"
	if formTitle != "" {
		subject = fmt.Sprintf("Registration received: %s", formTitle)
	}
	if settings != nil && settings.ResponseEmailSubject != nil {
		if s := strings.TrimSpace(*settings.ResponseEmailSubject); s != "" {
			subject = s
		}
	}

	templateKey := ""
	if settings != nil && settings.ResponseEmailTemplateKey != nil {
		templateKey = strings.Trim(strings.TrimSpace(*settings.ResponseEmailTemplateKey), "/")
	}
	templateImageURL := ""
	if settings != nil && settings.ResponseEmailTemplateURL != nil {
		templateImageURL = strings.TrimSpace(*settings.ResponseEmailTemplateURL)
	}
	// Backward compatibility: some saved forms may have a full URL in templateKey.
	if templateImageURL == "" && (strings.HasPrefix(strings.ToLower(templateKey), "http://") || strings.HasPrefix(strings.ToLower(templateKey), "https://")) {
		templateImageURL = strings.TrimSpace(templateKey)
		templateKey = ""
	}
	if templateKey == "" && form != nil && form.Slug != nil {
		slug := strings.TrimSpace(*form.Slug)
		if slug != "" {
			templateKey = "forms/" + slug
		}
	}

	formURL := ""
	if form != nil && form.Slug != nil {
		if u := s.buildPublicURL(*form.Slug); u != nil {
			formURL = *u
		}
	}

	subscribeURL := ""
	unsubscribeURL := ""
	if strings.TrimSpace(s.branding.PublicURL) != "" {
		subscribeURL = strings.TrimRight(s.branding.PublicURL, "/") + "/api/v1/notifications/subscribe?email=" + url.QueryEscape(addr)
		if recipient != "" {
			subscribeURL += "&name=" + url.QueryEscape(recipient)
		}
		unsubscribeURL = strings.TrimRight(s.branding.PublicURL, "/") + "/api/v1/notifications/unsubscribe?email=" + url.QueryEscape(addr)
	}

	code := ""
	if regCode != nil {
		code = strings.TrimSpace(*regCode)
	}

	calendarOptInURL := ""
	googleCalendarURL := ""
	calendarICSURL := ""
	if form != nil && form.Slug != nil {
		calendarOptInURL, googleCalendarURL, calendarICSURL = s.createCalendarReminder(
			form,
			event,
			addr,
			recipient,
			code,
			submissionID,
		)
	}

	hero := ""
	if templateImageURL != "" {
		hero = templateImageURL
	}
	if settings != nil && settings.Design != nil && settings.Design.CoverImageURL != nil {
		if hero == "" {
			hero = strings.TrimSpace(*settings.Design.CoverImageURL)
		}
	}
	if hero == "" && event.BannerImage != nil {
		hero = strings.TrimSpace(*event.BannerImage)
	}
	if hero == "" && event.Image != nil {
		hero = strings.TrimSpace(*event.Image)
	}

	now := time.Now().UTC()
	templateData := map[string]any{
		"Branding":          s.branding,
		"Form":              form,
		"Event":             event,
		"Values":            values,
		"RecipientName":     recipient,
		"FullName":          recipient,
		"Name":              recipient,
		"FirstName":         firstToken(recipient),
		"Email":             addr,
		"RegistrationCode":  code,
		"FormURL":           formURL,
		"PublicURL":         formURL,
		"SubscribeURL":      subscribeURL,
		"UnsubscribeURL":    unsubscribeURL,
		"CalendarOptInURL":  calendarOptInURL,
		"GoogleCalendarURL": googleCalendarURL,
		"CalendarICSURL":    calendarICSURL,
		"FormTitle":         formTitle,
		"EventTitle":        strings.TrimSpace(event.Title),
		"EventDate":         strings.TrimSpace(event.Date),
		"EventTime":         strings.TrimSpace(event.Time),
		"EventLocation":     strings.TrimSpace(event.Location),
		"HeroImageURL":      hero,
		"TemplateImageURL":  templateImageURL,
		"SubmittedAt":       now.Format(time.RFC3339),
		"SubmittedAtText":   now.Format("Mon, 02 Jan 2006 15:04 MST"),
		"Year":              now.Year(),
	}

	var body string

	if s.templateRepo != nil {
		var tpl *models.EmailTemplate
		if tpl == nil && settings != nil && settings.ResponseEmailTemplateID != nil {
			if t, err := s.templateRepo.GetByID(strings.TrimSpace(*settings.ResponseEmailTemplateID)); err == nil && t != nil {
				if t.Status != models.EmailTemplateArchived {
					tpl = t
				}
			}
		}
		if templateKey != "" {
			if t, err := s.templateRepo.GetActiveByKey(templateKey); err == nil && t != nil {
				tpl = t
			}
		}
		if tpl == nil && form != nil {
			if t, err := s.templateRepo.GetActiveByOwner("form", form.ID); err == nil && t != nil {
				tpl = t
			}
		}
		if tpl != nil {
			if rendered, err := renderDBTemplate(tpl, templateData); err == nil && strings.TrimSpace(rendered) != "" {
				body = rendered
				if tpl.Subject != nil && strings.TrimSpace(*tpl.Subject) != "" {
					subject = strings.TrimSpace(*tpl.Subject)
				}
			} else if err != nil {
				log.Printf("⚠️ response email DB template render failed (templateID=%s, formID=%s): %v", tpl.ID, formID, err)
			}
		}
	}

	if strings.TrimSpace(body) == "" && templateKey != "" && s.tplStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.templateTimeout)
		defer cancel()

		_, htmlOut, _, err := s.tplStore.RenderWithData(ctx, templateKey, templateData)
		if err == nil && strings.TrimSpace(htmlOut) != "" {
			body = htmlOut
		} else if err != nil {
			log.Printf("⚠️ response email remote template render failed (templateKey=%s, formID=%s): %v", templateKey, formID, err)
		}
	}

	if strings.TrimSpace(body) == "" {
		message := ""
		if settings != nil && settings.SuccessMessage != nil {
			message = strings.TrimSpace(*settings.SuccessMessage)
		}

		body = email.RenderFormResponseEmail(email.FormResponseTemplateData{
			Branding:          s.branding,
			RecipientName:     recipient,
			FormTitle:         formTitle,
			RegistrationCode:  code,
			Message:           message,
			HeroImageURL:      hero,
			CalendarOptInURL:  calendarOptInURL,
			GoogleCalendarURL: googleCalendarURL,
			CalendarICSURL:    calendarICSURL,
			SubscribeURL:      subscribeURL,
			UnsubscribeURL:    unsubscribeURL,
		})
	}

	if err := s.sender.SendHTML(addr, subject, body); err != nil {
		log.Printf("⚠️ failed to send form response email to %s (templateKey=%s, formID=%s): %v", addr, templateKey, formID, err)
	}
}

type formCampaignRequestView struct {
	Campaign             email.FormCampaignTemplateData
	IncludeCalendarLinks bool
	TargetSubmissionIDs  []string
	TemplateID           string
	TemplateKey          string
	Title                string
	HTMLBody             string
	TextBody             string
}

type formCampaignTemplateSelection struct {
	DBTemplate *models.EmailTemplate
	RemoteKey  string
	Source     string
}

type formCampaignRenderedContent struct {
	HTML string
	Text string
}

func (s *formService) SendFormCampaignEmail(formID string, req *models.SendFormCampaignEmailRequest, actor *models.FormCampaignSendActor) (*models.SendFormCampaignEmailResponse, error) {
	if s.sender == nil {
		return nil, errors.New("email sender is not configured")
	}
	if senderStatus, ok := s.sender.(interface{ DisabledReason() string }); ok {
		if reason := strings.TrimSpace(senderStatus.DisabledReason()); reason != "" {
			return nil, errors.New(reason)
		}
	}
	if strings.TrimSpace(formID) == "" {
		return nil, errors.New("form id is required")
	}
	if req == nil {
		return nil, errors.New("request is required")
	}

	form, err := s.repo.GetByID(strings.TrimSpace(formID))
	if err != nil {
		return nil, err
	}

	normalized, err := normalizeFormCampaignRequest(req)
	if err != nil {
		return nil, err
	}

	settings, _ := decodeSettings(form.Settings)

	templateSelection, err := s.resolveFormCampaignTemplate(normalized.TemplateID, normalized.TemplateKey)
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
	if templateSelection == nil && settings != nil {
		templateSelection, err = s.resolveFormCampaignTemplate(
			strings.TrimSpace(valueOrEmpty(settings.CampaignEmailTemplateID)),
			strings.Trim(strings.TrimSpace(valueOrEmpty(settings.CampaignEmailTemplateKey)), "/"),
		)
		if err != nil {
			return nil, err
		}
	}
	if templateSelection == nil && s.templateRepo != nil {
		if tpl, err := s.templateRepo.GetActiveByOwner("form_campaign", form.ID); err == nil && tpl != nil && tpl.Status != models.EmailTemplateArchived {
			templateSelection = &formCampaignTemplateSelection{
				DBTemplate: tpl,
				Source:     "db_owner:form_campaign",
			}
		}
	}
	if templateSelection == nil {
		templateSelection = s.resolveDefaultFormCampaignTemplate(form, settings)
	}

	submissions, err := s.repo.ListEmailSubmissions(form.ID)
	if err != nil {
		return nil, err
	}
	if len(submissions) == 0 {
		return nil, errors.New("no registered users with email found for this form")
	}
	if len(normalized.TargetSubmissionIDs) > 0 {
		targetedIDs := make(map[string]struct{}, len(normalized.TargetSubmissionIDs))
		for _, id := range normalized.TargetSubmissionIDs {
			targetedIDs[id] = struct{}{}
		}
		filtered := make([]models.FormSubmission, 0, len(normalized.TargetSubmissionIDs))
		for _, submission := range submissions {
			if _, ok := targetedIDs[strings.TrimSpace(submission.ID)]; ok {
				filtered = append(filtered, submission)
			}
		}
		submissions = filtered
		if len(submissions) == 0 {
			return nil, errors.New("no selected recipients matched this form")
		}
	}

	var event *models.Event
	if form.EventID != nil && s.eventRepo != nil {
		if ev, err := s.eventRepo.GetByID(strings.TrimSpace(*form.EventID)); err == nil && ev != nil {
			event = ev
		}
	}

	formTitle := strings.TrimSpace(form.Title)
	if formTitle == "" && event != nil {
		formTitle = strings.TrimSpace(event.Title)
	}

	formURL := ""
	if form != nil && form.Slug != nil {
		if u := s.buildPublicURL(*form.Slug); u != nil {
			formURL = *u
		}
	}

	startedAt := time.Now().UTC()
	subject := strings.TrimSpace(normalized.Campaign.Subject)
	if subject == "" && settings != nil && settings.CampaignEmailSubject != nil {
		subject = strings.TrimSpace(*settings.CampaignEmailSubject)
	}
	if subject == "" && templateSelection != nil && templateSelection.DBTemplate != nil && templateSelection.DBTemplate.Subject != nil {
		subject = strings.TrimSpace(*templateSelection.DBTemplate.Subject)
	}
	if subject == "" {
		subject = defaultFormCampaignSubject(formTitle, event)
	}

	templateSource := "built_in"
	if templateSelection != nil && strings.TrimSpace(templateSelection.Source) != "" {
		templateSource = strings.TrimSpace(templateSelection.Source)
	}

	resp := &models.SendFormCampaignEmailResponse{
		FormID:           form.ID,
		FormTitle:        formTitle,
		Subject:          subject,
		TemplateSource:   templateSource,
		StartedAt:        startedAt.Format(time.RFC3339),
		SentAt:           startedAt.Format(time.RFC3339),
		FailedRecipients: []string{},
	}
	if event != nil {
		if title := strings.TrimSpace(event.Title); title != "" {
			resp.EventTitle = &title
		}
	}

	seenEmails := make(map[string]struct{}, len(submissions))
	for i := range submissions {
		submission := submissions[i]
		addr := normalizeEmail(valueOrEmpty(submission.Email))
		if addr == "" {
			resp.Skipped++
			continue
		}
		if _, exists := seenEmails[addr]; exists {
			resp.Skipped++
			continue
		}
		seenEmails[addr] = struct{}{}
		resp.Targeted++

		values := decodeSubmissionValues(submission.Values)
		recipient := strings.TrimSpace(valueOrEmpty(submission.Name))
		if recipient == "" {
			recipient = strings.TrimSpace(valueAsString(values, "fullName", "full_name", "name", "firstName", "first_name"))
		}
		registrationCode := strings.TrimSpace(valueOrEmpty(submission.RegistrationCode))

		campaignData := normalized.Campaign
		campaignData.Branding = s.branding
		campaignData.Subject = subject
		campaignData.FormTitle = formTitle
		campaignData.RecipientName = recipient
		campaignData.RegistrationCode = registrationCode
		campaignData.HeroImageURL = pickFormCampaignHeroImage(campaignData.HeroImageURL, settings, event)
		if strings.TrimSpace(campaignData.HeroTitle) == "" && strings.TrimSpace(normalized.Title) != "" {
			campaignData.HeroTitle = strings.TrimSpace(normalized.Title)
		}
		if event != nil {
			campaignData.EventTitle = strings.TrimSpace(event.Title)
			campaignData.EventDate = strings.TrimSpace(event.Date)
			campaignData.EventTime = strings.TrimSpace(event.Time)
			campaignData.EventLocation = strings.TrimSpace(event.Location)
		}
		if campaignData.IntroHTML == "" && campaignData.BodyHTML == "" {
			campaignData.IntroHTML = defaultFormCampaignIntroHTML(formTitle, campaignData.EventTitle)
		}
		if campaignData.ClosingHTML == "" {
			campaignData.ClosingHTML = defaultFormCampaignClosingHTML(s.branding)
		}

		calendarOptInURL := ""
		googleCalendarURL := ""
		calendarICSURL := ""
		if normalized.IncludeCalendarLinks {
			calendarOptInURL, googleCalendarURL, calendarICSURL = s.getOrCreateCampaignCalendarLinks(
				form,
				event,
				&submission,
				addr,
				recipient,
				registrationCode,
			)
		}
		campaignData.CalendarOptInURL = calendarOptInURL
		campaignData.GoogleCalendarURL = googleCalendarURL
		campaignData.CalendarICSURL = calendarICSURL

		campaignTitle := strings.TrimSpace(campaignData.HeroTitle)
		if campaignTitle == "" {
			campaignTitle = strings.TrimSpace(normalized.Title)
		}
		if campaignTitle == "" {
			campaignTitle = formTitle
		}

		subscribeURL := ""
		unsubscribeURL := ""
		if strings.TrimSpace(s.branding.PublicURL) != "" {
			subscribeURL = strings.TrimRight(s.branding.PublicURL, "/") + "/api/v1/notifications/subscribe?email=" + url.QueryEscape(addr)
			if recipient != "" {
				subscribeURL += "&name=" + url.QueryEscape(recipient)
			}
			unsubscribeURL = strings.TrimRight(s.branding.PublicURL, "/") + "/api/v1/notifications/unsubscribe?email=" + url.QueryEscape(addr)
		}

		templateData := map[string]any{
			"Branding":          s.branding,
			"Campaign":          campaignData,
			"Form":              form,
			"Event":             event,
			"Submission":        submission,
			"Values":            values,
			"RecipientName":     recipient,
			"FullName":          recipient,
			"Name":              recipient,
			"FirstName":         firstToken(recipient),
			"Email":             addr,
			"CampaignTitle":     campaignTitle,
			"Title":             campaignTitle,
			"RegistrationCode":  registrationCode,
			"FormTitle":         formTitle,
			"FormURL":           formURL,
			"PublicURL":         formURL,
			"EventTitle":        campaignData.EventTitle,
			"EventDate":         campaignData.EventDate,
			"EventTime":         campaignData.EventTime,
			"EventLocation":     campaignData.EventLocation,
			"HeroEyebrow":       campaignData.HeroEyebrow,
			"HeroTitle":         campaignData.HeroTitle,
			"HeroSubtitle":      campaignData.HeroSubtitle,
			"IntroHTML":         campaignData.IntroHTML,
			"BodyHTML":          campaignData.BodyHTML,
			"ClosingHTML":       campaignData.ClosingHTML,
			"CalendarOptInURL":  calendarOptInURL,
			"GoogleCalendarURL": googleCalendarURL,
			"CalendarICSURL":    calendarICSURL,
			"SubscribeURL":      subscribeURL,
			"UnsubscribeURL":    unsubscribeURL,
			"Subject":           subject,
			"PreviewText":       campaignData.PreviewText,
			"HeroImageURL":      campaignData.HeroImageURL,
			"FlyerImageURLs":    campaignData.FlyerImageURLs,
			"Highlights":        campaignData.Highlights,
			"ResourceLinks":     campaignData.ResourceLinks,
			"PrimaryCTA":        campaignData.PrimaryCTA,
			"SecondaryCTA":      campaignData.SecondaryCTA,
			"FooterNote":        campaignData.FooterNote,
			"Year":              time.Now().UTC().Year(),
		}

		content, err := s.renderFormCampaignContent(templateSelection, campaignData, templateData)
		if err != nil {
			resp.Failed++
			resp.FailedRecipients = appendFailedRecipient(resp.FailedRecipients, addr)
			log.Printf("⚠️ failed to render form campaign email for %s (formID=%s): %v", addr, form.ID, err)
			continue
		}

		if err := s.sendFormCampaignMessage(addr, subject, content); err != nil {
			resp.Failed++
			resp.FailedRecipients = appendFailedRecipient(resp.FailedRecipients, addr)
			log.Printf("⚠️ failed to send form campaign email to %s (formID=%s): %v", addr, form.ID, err)
			continue
		}
		resp.Sent++
	}

	if resp.Targeted == 0 {
		return nil, errors.New("no valid recipient emails found for this form")
	}
	resp.TotalRecipients = resp.Targeted
	resp.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	resp.SentAt = resp.CompletedAt

	delivery, saveErr := s.saveFormCampaignDelivery(form, event, templateSelection, resp, actor)
	if saveErr != nil {
		log.Printf("⚠️ failed to persist form campaign delivery (formID=%s): %v", form.ID, saveErr)
	} else if delivery != nil {
		resp.DeliveryID = &delivery.ID
	}

	return resp, nil
}

func normalizeFormCampaignRequest(req *models.SendFormCampaignEmailRequest) (*formCampaignRequestView, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}

	subject, err := normalizeFormCampaignText("subject", req.Subject, 180)
	if err != nil {
		return nil, err
	}
	title, err := normalizeFormCampaignText("title", req.Title, 180)
	if err != nil {
		return nil, err
	}
	previewText, err := normalizeFormCampaignText("previewText", req.PreviewText, 220)
	if err != nil {
		return nil, err
	}
	heroEyebrow, err := normalizeFormCampaignText("heroEyebrow", req.HeroEyebrow, 80)
	if err != nil {
		return nil, err
	}
	heroTitle, err := normalizeFormCampaignText("heroTitle", req.HeroTitle, 160)
	if err != nil {
		return nil, err
	}
	heroSubtitle, err := normalizeFormCampaignText("heroSubtitle", req.HeroSubtitle, 280)
	if err != nil {
		return nil, err
	}
	footerNote, err := normalizeFormCampaignText("footerNote", req.FooterNote, 320)
	if err != nil {
		return nil, err
	}
	heroImageURL, err := normalizeFormCampaignURL("heroImageUrl", req.HeroImageURL)
	if err != nil {
		return nil, err
	}
	flyerImageURLs, err := normalizeFormCampaignImageURLs(req.FlyerImageURLs)
	if err != nil {
		return nil, err
	}
	primaryCTA, err := normalizeFormCampaignCTA("primaryCta", req.PrimaryCTA)
	if err != nil {
		return nil, err
	}
	secondaryCTA, err := normalizeFormCampaignCTA("secondaryCta", req.SecondaryCTA)
	if err != nil {
		return nil, err
	}
	highlights, err := normalizeFormCampaignHighlights(req.Highlights)
	if err != nil {
		return nil, err
	}
	resourceLinks, err := normalizeFormCampaignResources(req.ResourceLinks)
	if err != nil {
		return nil, err
	}
	targetSubmissionIDs := normalizeFormCampaignSubmissionIDs(req.TargetSubmissionIDs)
	htmlBody := strings.TrimSpace(valueOrEmpty(req.HTMLBody))
	textBody := strings.TrimSpace(valueOrEmpty(req.TextBody))

	templateID := strings.TrimSpace(valueOrEmpty(req.TemplateID))
	templateKey := strings.Trim(strings.TrimSpace(valueOrEmpty(req.TemplateKey)), "/")
	if templateID != "" && templateKey != "" {
		return nil, errors.New("templateId and templateKey cannot be used together")
	}

	includeCalendarLinks := true
	if req.IncludeCalendarLinks != nil {
		includeCalendarLinks = *req.IncludeCalendarLinks
	}

	view := &formCampaignRequestView{
		Campaign: email.FormCampaignTemplateData{
			Subject:        subject,
			PreviewText:    previewText,
			HeroEyebrow:    heroEyebrow,
			HeroTitle:      heroTitle,
			HeroSubtitle:   heroSubtitle,
			IntroHTML:      richTextToHTML(valueOrEmpty(req.IntroHTML)),
			BodyHTML:       richTextToHTML(valueOrEmpty(req.BodyHTML)),
			ClosingHTML:    richTextToHTML(valueOrEmpty(req.ClosingHTML)),
			HeroImageURL:   heroImageURL,
			FlyerImageURLs: flyerImageURLs,
			Highlights:     highlights,
			ResourceLinks:  resourceLinks,
			PrimaryCTA:     primaryCTA,
			SecondaryCTA:   secondaryCTA,
			FooterNote:     footerNote,
		},
		IncludeCalendarLinks: includeCalendarLinks,
		TargetSubmissionIDs:  targetSubmissionIDs,
		TemplateID:           templateID,
		TemplateKey:          templateKey,
		Title:                title,
		HTMLBody:             htmlBody,
		TextBody:             textBody,
	}

	return view, nil
}

func normalizeFormCampaignText(name string, value *string, maxLen int) (string, error) {
	raw := strings.TrimSpace(valueOrEmpty(value))
	if raw == "" {
		return "", nil
	}
	if maxLen > 0 && utf8.RuneCountInString(raw) > maxLen {
		return "", fmt.Errorf("%s must be %d characters or fewer", name, maxLen)
	}
	return raw, nil
}

func normalizeFormCampaignURL(name string, value *string) (string, error) {
	raw := strings.TrimSpace(valueOrEmpty(value))
	if raw == "" {
		return "", nil
	}
	normalized, err := normalizeAbsoluteTemplateURL(raw)
	if err != nil {
		return "", fmt.Errorf("%s must be a valid absolute URL", name)
	}
	return normalized, nil
}

func normalizeFormCampaignImageURLs(values *[]string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	clean := make([]string, 0, len(*values))
	for i, raw := range *values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		normalized, err := normalizeAbsoluteTemplateURL(trimmed)
		if err != nil {
			return nil, fmt.Errorf("flyerImageUrls[%d] must be a valid absolute URL", i)
		}
		clean = append(clean, normalized)
	}
	return clean, nil
}

func normalizeFormCampaignCTA(name string, input *models.FormCampaignEmailCTA) (*email.FormCampaignEmailCTA, error) {
	if input == nil {
		return nil, nil
	}
	label := strings.TrimSpace(input.Label)
	urlValue := strings.TrimSpace(input.URL)
	if label == "" && urlValue == "" {
		return nil, nil
	}
	if label == "" || urlValue == "" {
		return nil, fmt.Errorf("%s requires both label and url", name)
	}
	if utf8.RuneCountInString(label) > 80 {
		return nil, fmt.Errorf("%s.label must be 80 characters or fewer", name)
	}
	normalizedURL, err := normalizeAbsoluteTemplateURL(urlValue)
	if err != nil {
		return nil, fmt.Errorf("%s.url must be a valid absolute URL", name)
	}
	return &email.FormCampaignEmailCTA{
		Label: label,
		URL:   normalizedURL,
	}, nil
}

func normalizeFormCampaignHighlights(items *[]models.FormCampaignEmailHighlight) ([]email.FormCampaignEmailHighlight, error) {
	if items == nil {
		return nil, nil
	}
	highlights := make([]email.FormCampaignEmailHighlight, 0, len(*items))
	for i, item := range *items {
		label := strings.TrimSpace(item.Label)
		value := strings.TrimSpace(item.Value)
		if label == "" && value == "" {
			continue
		}
		if label == "" || value == "" {
			return nil, fmt.Errorf("highlights[%d] requires both label and value", i)
		}
		if utf8.RuneCountInString(label) > 80 {
			return nil, fmt.Errorf("highlights[%d].label must be 80 characters or fewer", i)
		}
		if utf8.RuneCountInString(value) > 200 {
			return nil, fmt.Errorf("highlights[%d].value must be 200 characters or fewer", i)
		}
		highlights = append(highlights, email.FormCampaignEmailHighlight{
			Label: label,
			Value: value,
		})
	}
	return highlights, nil
}

func normalizeFormCampaignResources(items *[]models.FormCampaignEmailResource) ([]email.FormCampaignEmailResource, error) {
	if items == nil {
		return nil, nil
	}
	resources := make([]email.FormCampaignEmailResource, 0, len(*items))
	for i, item := range *items {
		label := strings.TrimSpace(item.Label)
		urlValue := strings.TrimSpace(item.URL)
		description := strings.TrimSpace(valueOrEmpty(item.Description))
		kind := normalizeFormCampaignResourceKind(valueOrEmpty(item.Kind))
		if label == "" && urlValue == "" && description == "" {
			continue
		}
		if label == "" || urlValue == "" {
			return nil, fmt.Errorf("resourceLinks[%d] requires both label and url", i)
		}
		if utf8.RuneCountInString(label) > 80 {
			return nil, fmt.Errorf("resourceLinks[%d].label must be 80 characters or fewer", i)
		}
		if utf8.RuneCountInString(description) > 200 {
			return nil, fmt.Errorf("resourceLinks[%d].description must be 200 characters or fewer", i)
		}
		normalizedURL, err := normalizeAbsoluteTemplateURL(urlValue)
		if err != nil {
			return nil, fmt.Errorf("resourceLinks[%d].url must be a valid absolute URL", i)
		}
		resources = append(resources, email.FormCampaignEmailResource{
			Label:       label,
			URL:         normalizedURL,
			Description: description,
			Kind:        kind,
		})
	}
	return resources, nil
}

func normalizeFormCampaignResourceKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "flyer":
		return "flyer"
	case "document":
		return "document"
	case "guide":
		return "guide"
	case "schedule":
		return "schedule"
	default:
		return "resource"
	}
}

func normalizeFormCampaignSubmissionIDs(items *[]string) []string {
	if items == nil {
		return nil
	}
	ids := make([]string, 0, len(*items))
	seen := make(map[string]struct{}, len(*items))
	for _, raw := range *items {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func appendFailedRecipient(existing []string, emailAddr string) []string {
	addr := strings.TrimSpace(emailAddr)
	if addr == "" {
		return existing
	}
	for _, current := range existing {
		if strings.EqualFold(strings.TrimSpace(current), addr) {
			return existing
		}
	}
	return append(existing, addr)
}

func richTextToHTML(raw string) template.HTML {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "<") && strings.Contains(trimmed, ">") {
		return template.HTML(trimmed)
	}

	blocks := strings.Split(trimmed, "\n\n")
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		line := strings.TrimSpace(block)
		if line == "" {
			continue
		}
		escaped := template.HTMLEscapeString(line)
		escaped = strings.ReplaceAll(escaped, "\n", "<br>")
		parts = append(parts, "<p style=\"margin:0 0 16px;\">"+escaped+"</p>")
	}
	if len(parts) == 0 {
		return ""
	}
	return template.HTML(strings.Join(parts, ""))
}

func defaultFormCampaignSubject(formTitle string, event *models.Event) string {
	title := strings.TrimSpace(formTitle)
	if event != nil && strings.TrimSpace(event.Title) != "" {
		title = strings.TrimSpace(event.Title)
	}
	if title == "" {
		return "Important event update"
	}
	return "Mark your calendar: " + title
}

func defaultFormCampaignIntroHTML(formTitle string, eventTitle string) template.HTML {
	title := strings.TrimSpace(eventTitle)
	if title == "" {
		title = strings.TrimSpace(formTitle)
	}
	if title == "" {
		title = "our upcoming event"
	}

	return richTextToHTML(
		"We are reaching out with an important reminder about " + title + ".\n\n" +
			"Please open your calendar now, save the date, and keep these details close so the event is already on your schedule.",
	)
}

func defaultFormCampaignClosingHTML(branding email.Branding) template.HTML {
	brandName := strings.TrimSpace(branding.AppName)
	if brandName == "" {
		brandName = "the team"
	}
	return richTextToHTML("We look forward to receiving you.\n\nWarm regards,\n" + brandName)
}

func pickFormCampaignHeroImage(requestHero string, settings *models.FormSettingsDTO, event *models.Event) string {
	hero := strings.TrimSpace(requestHero)
	if hero != "" {
		return hero
	}
	if settings != nil && settings.CampaignEmailTemplateURL != nil {
		if v := strings.TrimSpace(*settings.CampaignEmailTemplateURL); v != "" {
			return v
		}
	}
	if settings != nil && settings.Design != nil && settings.Design.CoverImageURL != nil {
		if v := strings.TrimSpace(*settings.Design.CoverImageURL); v != "" {
			return v
		}
	}
	if event != nil && event.BannerImage != nil {
		if v := strings.TrimSpace(*event.BannerImage); v != "" {
			return v
		}
	}
	if event != nil && event.Image != nil {
		if v := strings.TrimSpace(*event.Image); v != "" {
			return v
		}
	}
	return ""
}

func (s *formService) resolveFormCampaignTemplate(templateID, templateKey string) (*formCampaignTemplateSelection, error) {
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

func (s *formService) resolveDefaultFormCampaignTemplate(form *models.Form, settings *models.FormSettingsDTO) *formCampaignTemplateSelection {
	if form == nil {
		return nil
	}

	candidates := make([]string, 0, 6)
	if settings != nil && settings.CampaignEmailTemplateKey != nil {
		if key := strings.Trim(strings.TrimSpace(*settings.CampaignEmailTemplateKey), "/"); key != "" {
			candidates = append(candidates, key)
		}
	}
	if strings.TrimSpace(form.ID) != "" {
		candidates = append(candidates,
			"forms/"+form.ID+"/campaigns/primary",
			"forms/"+form.ID+"/campaign",
		)
	}
	if titleKey := normalizeFormCampaignTemplateSegment(form.Title); titleKey != "" {
		candidates = append(candidates,
			"forms/"+titleKey+"/campaigns/primary",
			"forms/"+titleKey+"/campaign",
		)
	}
	if form.Slug != nil {
		if slug := strings.TrimSpace(*form.Slug); slug != "" {
			candidates = append(candidates,
				"forms/"+slug+"/campaigns/primary",
				"forms/"+slug+"/campaign",
			)
		}
	}

	seen := map[string]struct{}{}
	var remoteCandidate *formCampaignTemplateSelection
	for _, templateKey := range candidates {
		if templateKey == "" {
			continue
		}
		if _, exists := seen[templateKey]; exists {
			continue
		}
		seen[templateKey] = struct{}{}
		if s.templateRepo != nil {
			if tpl, err := s.templateRepo.GetActiveByKey(templateKey); err == nil && tpl != nil && tpl.Status != models.EmailTemplateArchived {
				return &formCampaignTemplateSelection{
					DBTemplate: tpl,
					Source:     "db_key:" + templateKey,
				}
			}
		}
		if remoteCandidate == nil && s.tplStore != nil {
			remoteCandidate = &formCampaignTemplateSelection{
				RemoteKey: templateKey,
				Source:    "remote:" + templateKey,
			}
		}
	}
	return remoteCandidate
}

func normalizeFormCampaignTemplateSegment(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	value = slugInvalidRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	value = slugDashCollapseRe.ReplaceAllString(value, "-")
	return value
}

func (s *formService) renderFormCampaignContent(selection *formCampaignTemplateSelection, campaign email.FormCampaignTemplateData, templateData map[string]any) (*formCampaignRenderedContent, error) {
	if selection == nil {
		htmlBody := email.RenderFormCampaignEmail(campaign)
		return &formCampaignRenderedContent{
			HTML: htmlBody,
			Text: stripHTMLToText(htmlBody),
		}, nil
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
	htmlBody := email.RenderFormCampaignEmail(campaign)
	return &formCampaignRenderedContent{
		HTML: htmlBody,
		Text: stripHTMLToText(htmlBody),
	}, nil
}

func (s *formService) sendFormCampaignMessage(to, subject string, content *formCampaignRenderedContent) error {
	if content == nil {
		return errors.New("rendered campaign content is nil")
	}
	htmlBody := strings.TrimSpace(content.HTML)
	if htmlBody == "" {
		return errors.New("rendered campaign html body is empty")
	}

	textBody := strings.TrimSpace(content.Text)
	if sender, ok := s.sender.(interface {
		SendHTMLText(to, subject, htmlBody, textBody string) error
	}); ok && textBody != "" {
		return sender.SendHTMLText(to, subject, htmlBody, textBody)
	}

	return s.sender.SendHTML(to, subject, htmlBody)
}

func (s *formService) saveFormCampaignDelivery(
	form *models.Form,
	event *models.Event,
	selection *formCampaignTemplateSelection,
	resp *models.SendFormCampaignEmailResponse,
	actor *models.FormCampaignSendActor,
) (*models.FormCampaignDelivery, error) {
	if form == nil || resp == nil {
		return nil, errors.New("form campaign delivery payload is incomplete")
	}

	startedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(resp.StartedAt))
	if err != nil {
		return nil, err
	}

	var completedAt *time.Time
	if value := strings.TrimSpace(resp.CompletedAt); value != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, value); parseErr == nil {
			completedAt = &parsed
		} else {
			return nil, parseErr
		}
	}

	templateID, templateKey := extractFormCampaignTemplateReference(selection)
	delivery := &models.FormCampaignDelivery{
		FormID:           form.ID,
		FormTitle:        strings.TrimSpace(resp.FormTitle),
		EventID:          form.EventID,
		EventTitle:       resp.EventTitle,
		Subject:          strings.TrimSpace(resp.Subject),
		TemplateSource:   strings.TrimSpace(resp.TemplateSource),
		TemplateID:       templateID,
		TemplateKey:      templateKey,
		Status:           deriveFormCampaignDeliveryStatus(resp),
		TotalRecipients:  resp.TotalRecipients,
		Targeted:         resp.Targeted,
		Sent:             resp.Sent,
		Skipped:          resp.Skipped,
		Failed:           resp.Failed,
		FailedRecipients: encodeStringListJSON(resp.FailedRecipients),
		StartedAt:        startedAt.UTC(),
		CompletedAt:      completedAt,
	}

	if event != nil && event.ID != "" {
		delivery.EventID = &event.ID
	}
	applyFormCampaignDeliveryActor(delivery, actor)

	if err := s.repo.CreateCampaignDelivery(delivery); err != nil {
		return nil, err
	}
	return delivery, nil
}

func applyFormCampaignDeliveryActor(delivery *models.FormCampaignDelivery, actor *models.FormCampaignSendActor) {
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

func deriveFormCampaignDeliveryStatus(resp *models.SendFormCampaignEmailResponse) models.FormCampaignDeliveryStatus {
	if resp == nil {
		return models.FormCampaignDeliveryFailed
	}
	switch {
	case resp.Failed <= 0:
		return models.FormCampaignDeliveryCompleted
	case resp.Sent > 0:
		return models.FormCampaignDeliveryPartial
	default:
		return models.FormCampaignDeliveryFailed
	}
}

func extractFormCampaignTemplateReference(selection *formCampaignTemplateSelection) (*string, *string) {
	if selection == nil {
		return nil, nil
	}
	if selection.DBTemplate != nil {
		return cleanOptionalString(selection.DBTemplate.ID), cleanOptionalString(selection.DBTemplate.TemplateKey)
	}
	if selection.RemoteKey != "" {
		return nil, cleanOptionalString(selection.RemoteKey)
	}
	return nil, nil
}

func (s *formService) getOrCreateCampaignCalendarLinks(
	form *models.Form,
	event *models.Event,
	submission *models.FormSubmission,
	emailAddr string,
	recipientName string,
	registrationCode string,
) (string, string, string) {
	if s.reminderRepo == nil || form == nil || event == nil || submission == nil {
		return "", "", ""
	}
	if strings.TrimSpace(emailAddr) == "" || strings.TrimSpace(submission.ID) == "" {
		return "", "", ""
	}

	if existing, err := s.reminderRepo.GetBySubmissionID(submission.ID); err == nil && existing != nil {
		s.syncCampaignReminder(existing, form, event, submission, emailAddr, recipientName, registrationCode)
		return s.buildCalendarLinksFromReminder(existing)
	}

	optInURL, googleURL, icsURL := s.createCalendarReminder(
		form,
		event,
		emailAddr,
		recipientName,
		registrationCode,
		submission.ID,
	)
	if optInURL != "" || googleURL != "" || icsURL != "" {
		return optInURL, googleURL, icsURL
	}

	if existing, err := s.reminderRepo.GetBySubmissionID(submission.ID); err == nil && existing != nil {
		return s.buildCalendarLinksFromReminder(existing)
	}

	return "", "", ""
}

func (s *formService) buildCalendarLinksFromReminder(item *models.FormCalendarReminder) (string, string, string) {
	if item == nil {
		return "", "", ""
	}
	slug := strings.TrimSpace(item.Slug)
	token := strings.TrimSpace(item.CalendarToken)
	if slug == "" || token == "" {
		return "", "", ""
	}

	location := valueOrEmpty(item.EventLocation)
	endAt := item.EventStartsAt.UTC().Add(2 * time.Hour)
	if item.EventEndsAt != nil && item.EventEndsAt.After(item.EventStartsAt) {
		endAt = item.EventEndsAt.UTC()
	}

	optInURL := s.buildCalendarOptInURL(slug, token)
	icsURL := s.buildCalendarICSURL(slug, token)
	googleURL := buildGoogleCalendarURL(
		item.EventTitle,
		location,
		buildCalendarDetails(item.EventTitle, valueOrEmpty(item.RegistrationCode), s.branding.AppName),
		item.EventStartsAt.UTC(),
		endAt,
	)

	return optInURL, googleURL, icsURL
}

func (s *formService) createCalendarReminder(form *models.Form, event *models.Event, emailAddr, recipientName, registrationCode, submissionID string) (string, string, string) {
	if s.reminderRepo == nil || form == nil || event == nil {
		return "", "", ""
	}
	if strings.TrimSpace(emailAddr) == "" || strings.TrimSpace(submissionID) == "" {
		return "", "", ""
	}
	if form.Slug == nil {
		return "", "", ""
	}
	slug := strings.TrimSpace(*form.Slug)
	if slug == "" {
		return "", "", ""
	}

	eventDate := strings.TrimSpace(event.Date)
	if eventDate == "" {
		return "", "", ""
	}

	startAt, endAt, err := parseEventSchedule(eventDate, event.Time)
	if err != nil {
		return "", "", ""
	}

	token, err := generateSecureToken(24)
	if err != nil {
		log.Printf("⚠️ calendar token generation failed (formID=%s, submissionID=%s): %v", form.ID, submissionID, err)
		return "", "", ""
	}

	eventTitle := strings.TrimSpace(event.Title)
	if eventTitle == "" {
		eventTitle = strings.TrimSpace(form.Title)
	}
	if eventTitle == "" {
		eventTitle = "Church Event"
	}

	location := strings.TrimSpace(event.Location)
	var recipientPtr *string
	if recipientName != "" {
		name := recipientName
		recipientPtr = &name
	}
	var codePtr *string
	if registrationCode != "" {
		code := registrationCode
		codePtr = &code
	}
	var locationPtr *string
	if location != "" {
		loc := location
		locationPtr = &loc
	}
	endAtCopy := endAt

	item := &models.FormCalendarReminder{
		FormID:           form.ID,
		SubmissionID:     submissionID,
		Slug:             slug,
		Email:            emailAddr,
		RecipientName:    recipientPtr,
		RegistrationCode: codePtr,
		EventTitle:       eventTitle,
		EventLocation:    locationPtr,
		EventDate:        eventDate,
		EventTime:        strings.TrimSpace(event.Time),
		EventStartsAt:    startAt.UTC(),
		EventEndsAt:      &endAtCopy,
		CalendarToken:    token,
	}

	if err := s.reminderRepo.Create(item); err != nil {
		if existing, getErr := s.reminderRepo.GetBySubmissionID(submissionID); getErr == nil && existing != nil {
			return s.buildCalendarLinksFromReminder(existing)
		}
		log.Printf("⚠️ failed to persist calendar reminder (formID=%s, submissionID=%s): %v", form.ID, submissionID, err)
		return "", "", ""
	}

	optInURL := s.buildCalendarOptInURL(slug, token)
	icsURL := s.buildCalendarICSURL(slug, token)
	details := buildCalendarDetails(item.EventTitle, registrationCode, s.branding.AppName)
	googleURL := buildGoogleCalendarURL(item.EventTitle, location, details, item.EventStartsAt, endAt)

	return optInURL, googleURL, icsURL
}

func parseEventSchedule(dateValue string, timeValue string) (time.Time, time.Time, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(dateValue))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	hour, minute := 9, 0
	clock := strings.TrimSpace(timeValue)
	if clock != "" {
		h, m, parseErr := parseEventClock(clock)
		if parseErr == nil {
			hour, minute = h, m
		}
	}

	start := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	return start, end, nil
}

func parseEventClock(value string) (int, int, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return 9, 0, nil
	}
	compact := strings.ToUpper(strings.ReplaceAll(clean, " ", ""))
	layouts := []string{"15:04", "15:04:05", "3PM", "3:04PM", "3:04:05PM"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, compact); err == nil {
			return t.Hour(), t.Minute(), nil
		}
	}
	return 0, 0, errors.New("invalid event time")
}

func generateSecureToken(size int) (string, error) {
	if size <= 0 {
		size = 24
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *formService) buildCalendarOptInURL(slug, token string) string {
	base := strings.TrimRight(strings.TrimSpace(s.branding.PublicURL), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf(
		"%s/api/v1/forms/%s/calendar/confirm?token=%s",
		base,
		url.PathEscape(strings.TrimSpace(slug)),
		url.QueryEscape(strings.TrimSpace(token)),
	)
}

func (s *formService) buildCalendarICSURL(slug, token string) string {
	base := strings.TrimRight(strings.TrimSpace(s.branding.PublicURL), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf(
		"%s/api/v1/forms/%s/calendar.ics?token=%s",
		base,
		url.PathEscape(strings.TrimSpace(slug)),
		url.QueryEscape(strings.TrimSpace(token)),
	)
}

func buildGoogleCalendarURL(title, location, details string, startsAt, endsAt time.Time) string {
	if startsAt.IsZero() {
		return ""
	}
	if endsAt.IsZero() || !endsAt.After(startsAt) {
		endsAt = startsAt.Add(2 * time.Hour)
	}

	q := url.Values{}
	q.Set("action", "TEMPLATE")
	q.Set("text", strings.TrimSpace(title))
	q.Set("dates", startsAt.UTC().Format("20060102T150405Z")+"/"+endsAt.UTC().Format("20060102T150405Z"))
	if strings.TrimSpace(details) != "" {
		q.Set("details", strings.TrimSpace(details))
	}
	if strings.TrimSpace(location) != "" {
		q.Set("location", strings.TrimSpace(location))
	}

	return "https://calendar.google.com/calendar/render?" + q.Encode()
}

func buildCalendarDetails(eventTitle, registrationCode, appName string) string {
	title := strings.TrimSpace(eventTitle)
	if title == "" {
		title = "Church Event"
	}
	brand := strings.TrimSpace(appName)
	if brand == "" {
		brand = "Wisdom House"
	}

	if strings.TrimSpace(registrationCode) == "" {
		return fmt.Sprintf("%s - %s", brand, title)
	}
	return fmt.Sprintf("%s - %s (Registration: %s)", brand, title, strings.TrimSpace(registrationCode))
}

func (s *formService) ConfirmCalendarOptIn(slug, token string) (*models.FormCalendarPayload, error) {
	if s.reminderRepo == nil {
		return nil, errors.New("calendar reminders not configured")
	}
	trimmedSlug := strings.Trim(strings.TrimSpace(slug), "/")
	trimmedToken := strings.TrimSpace(token)
	if trimmedSlug == "" || trimmedToken == "" {
		return nil, errors.New("invalid calendar link")
	}

	row, err := s.reminderRepo.GetBySlugAndToken(trimmedSlug, trimmedToken)
	if err != nil {
		return nil, err
	}

	if err := s.reminderRepo.MarkOptedIn(row.ID, time.Now().UTC()); err != nil {
		log.Printf("⚠️ failed to mark calendar opt-in (id=%s): %v", row.ID, err)
	}

	location := ""
	if row.EventLocation != nil {
		location = strings.TrimSpace(*row.EventLocation)
	}

	endAt := row.EventStartsAt.Add(2 * time.Hour)
	if row.EventEndsAt != nil {
		endAt = row.EventEndsAt.UTC()
	}

	return &models.FormCalendarPayload{
		EventTitle:    row.EventTitle,
		EventDate:     row.EventDate,
		EventTime:     row.EventTime,
		EventLocation: location,
		GoogleURL:     buildGoogleCalendarURL(row.EventTitle, location, buildCalendarDetails(row.EventTitle, valueOrEmpty(row.RegistrationCode), s.branding.AppName), row.EventStartsAt.UTC(), endAt),
		ICSURL:        s.buildCalendarICSURL(trimmedSlug, trimmedToken),
	}, nil
}

func (s *formService) BuildCalendarICS(slug, token string) (string, []byte, error) {
	if s.reminderRepo == nil {
		return "", nil, errors.New("calendar reminders not configured")
	}
	trimmedSlug := strings.Trim(strings.TrimSpace(slug), "/")
	trimmedToken := strings.TrimSpace(token)
	if trimmedSlug == "" || trimmedToken == "" {
		return "", nil, errors.New("invalid calendar link")
	}

	row, err := s.reminderRepo.GetBySlugAndToken(trimmedSlug, trimmedToken)
	if err != nil {
		return "", nil, err
	}

	filename := sanitizeCalendarFilename(row.EventTitle)
	content := buildICSContent(row, s.branding.AppName)
	return filename, []byte(content), nil
}

func buildICSContent(row *models.FormCalendarReminder, appName string) string {
	if row == nil {
		return ""
	}
	start := row.EventStartsAt.UTC()
	end := start.Add(2 * time.Hour)
	if row.EventEndsAt != nil && row.EventEndsAt.After(start) {
		end = row.EventEndsAt.UTC()
	}

	location := ""
	if row.EventLocation != nil {
		location = *row.EventLocation
	}
	description := buildCalendarDetails(row.EventTitle, valueOrEmpty(row.RegistrationCode), appName)

	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Wisdom House//Event Registration//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + escapeICS(row.ID) + "@wisdomchurchhq.org",
		"DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z"),
		"DTSTART:" + start.Format("20060102T150405Z"),
		"DTEND:" + end.Format("20060102T150405Z"),
		"SUMMARY:" + escapeICS(row.EventTitle),
	}
	if strings.TrimSpace(location) != "" {
		lines = append(lines, "LOCATION:"+escapeICS(location))
	}
	if strings.TrimSpace(description) != "" {
		lines = append(lines, "DESCRIPTION:"+escapeICS(description))
	}
	lines = append(lines,
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	)
	return strings.Join(lines, "\r\n")
}

func escapeICS(value string) string {
	v := strings.TrimSpace(value)
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, ";", "\\;")
	v = strings.ReplaceAll(v, ",", "\\,")
	v = strings.ReplaceAll(v, "\r\n", "\\n")
	v = strings.ReplaceAll(v, "\n", "\\n")
	return v
}

func sanitizeCalendarFilename(title string) string {
	name := strings.ToLower(strings.TrimSpace(title))
	if name == "" {
		name = "event"
	}
	name = strings.ReplaceAll(name, " ", "-")
	name = slugInvalidRe.ReplaceAllString(name, "")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "event"
	}
	return name + ".ics"
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func (s *formService) SendEventReminderEmails(now time.Time, lookAhead time.Duration) (int, int, error) {
	if s.reminderRepo == nil || s.sender == nil {
		return 0, 0, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if lookAhead <= 0 {
		lookAhead = 24 * time.Hour
	}

	rows, err := s.reminderRepo.ListDue(now.UTC(), now.UTC().Add(lookAhead), 500)
	if err != nil {
		return 0, 0, err
	}

	sent := 0
	failed := 0
	for i := range rows {
		item := rows[i]
		addr := strings.TrimSpace(item.Email)
		if addr == "" {
			failed++
			continue
		}

		location := valueOrEmpty(item.EventLocation)
		regCode := valueOrEmpty(item.RegistrationCode)
		icsURL := s.buildCalendarICSURL(item.Slug, item.CalendarToken)
		googleURL := buildGoogleCalendarURL(
			item.EventTitle,
			location,
			buildCalendarDetails(item.EventTitle, regCode, s.branding.AppName),
			item.EventStartsAt.UTC(),
			func() time.Time {
				if item.EventEndsAt != nil {
					return item.EventEndsAt.UTC()
				}
				return item.EventStartsAt.UTC().Add(2 * time.Hour)
			}(),
		)

		subject := "Gentle reminder: " + strings.TrimSpace(item.EventTitle) + " is tomorrow"
		body := email.RenderEventReminderEmail(email.EventReminderTemplateData{
			Branding:          s.branding,
			RecipientName:     valueOrEmpty(item.RecipientName),
			EventTitle:        item.EventTitle,
			EventDate:         item.EventDate,
			EventTime:         item.EventTime,
			EventLocation:     location,
			RegistrationCode:  regCode,
			GoogleCalendarURL: googleURL,
			CalendarICSURL:    icsURL,
			UnsubscribeURL:    strings.TrimRight(strings.TrimSpace(s.branding.PublicURL), "/") + "/api/v1/notifications/unsubscribe?email=" + url.QueryEscape(addr),
		})

		if err := s.sender.SendHTML(addr, subject, body); err != nil {
			failed++
			log.Printf("⚠️ failed to send event reminder email to %s: %v", addr, err)
			continue
		}
		if err := s.reminderRepo.MarkReminderSent(item.ID, now.UTC()); err != nil {
			failed++
			log.Printf("⚠️ failed to mark event reminder sent (id=%s): %v", item.ID, err)
			continue
		}
		sent++
	}

	return sent, failed, nil
}

func renderDBTemplate(tpl *models.EmailTemplate, data any) (string, error) {
	if tpl == nil {
		return "", errors.New("template is nil")
	}
	raw := strings.TrimSpace(tpl.HTMLBody)
	if raw == "" {
		return "", errors.New("template html is empty")
	}
	t, err := template.New("db").Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderDBTextTemplate(tpl *models.EmailTemplate, data any) (string, error) {
	if tpl == nil || tpl.TextBody == nil {
		return "", nil
	}
	raw := strings.TrimSpace(*tpl.TextBody)
	if raw == "" {
		return "", nil
	}
	t, err := texttemplate.New("db_text").Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func encodeStringListJSON(items []string) datatypes.JSON {
	clean := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		clean = append(clean, value)
	}
	if len(clean) == 0 {
		return datatypes.JSON([]byte("[]"))
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return datatypes.JSON([]byte("[]"))
	}
	return datatypes.JSON(raw)
}

func decodeStringListJSON(raw datatypes.JSON) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return []string{}
	}
	clean := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		clean = append(clean, value)
	}
	return clean
}

func formatOptionalTimeRFC3339(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func cleanOptionalString(value string) *string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return nil
	}
	return &clean
}

func stripHTMLToText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	normalized := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</p>", "\n\n",
		"</div>", "\n",
		"</li>", "\n",
		"&nbsp;", " ",
	).Replace(trimmed)
	withoutTags := regexp.MustCompile(`(?s)<[^>]*>`).ReplaceAllString(normalized, "")
	lines := strings.Split(withoutTags, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if line == "" {
			if len(clean) == 0 || clean[len(clean)-1] == "" {
				continue
			}
		}
		clean = append(clean, line)
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
}

func (s *formService) syncSubmissionTarget(form *models.Form, settings *models.FormSettingsDTO, values map[string]any) error {
	if settings == nil || settings.SubmissionTarget == nil {
		return nil
	}
	target := strings.ToLower(strings.TrimSpace(*settings.SubmissionTarget))
	switch target {
	case "workforce":
		fallthrough
	case "workforce_new":
		if s.workforceSvc == nil {
			return errors.New("workforce service not configured")
		}
		req, err := buildWorkforceRequest(values, settings, false)
		if err != nil {
			return err
		}
		_, err = s.workforceSvc.CreateApplication(req)
		return err
	case "workforce_serving":
		if s.workforceSvc == nil {
			return errors.New("workforce service not configured")
		}
		req, err := buildWorkforceRequest(values, settings, true)
		if err != nil {
			return err
		}
		_, err = s.workforceSvc.RegisterExisting(req)
		return err
	case "member":
		if s.memberSvc == nil {
			return errors.New("member service not configured")
		}
		req, err := buildMemberRequest(values)
		if err != nil {
			return err
		}
		_, err = s.memberSvc.Create(req)
		return err
	case "leadership":
		if s.leadershipSvc == nil {
			return errors.New("leadership service not configured")
		}
		req, err := buildLeadershipRequest(values)
		if err != nil {
			return err
		}
		_, err = s.leadershipSvc.Apply(req)
		return err
	case "testimonial":
		if s.testimonialSvc == nil {
			return errors.New("testimonial service not configured")
		}
		req, err := buildTestimonialRequest(values)
		if err != nil {
			return err
		}
		_, err = s.testimonialSvc.CreateTestimonial(req)
		return err
	default:
		return nil
	}
}

func buildWorkforceRequest(values map[string]any, settings *models.FormSettingsDTO, existing bool) (*models.CreateWorkforceRequest, error) {
	first := valueAsString(values, "firstName", "first_name", "firstname", "givenName")
	last := valueAsString(values, "lastName", "last_name", "lastname", "surname", "familyName")
	if first == "" || last == "" {
		full := valueAsString(values, "fullName", "full_name", "name")
		if full != "" {
			f, l := splitName(full)
			if first == "" {
				first = f
			}
			if last == "" {
				last = l
			}
		}
	}

	dept := valueAsString(values, "department", "dept", "ministry", "unit")
	if dept == "" && settings != nil && settings.SubmissionDepartment != nil {
		dept = strings.TrimSpace(*settings.SubmissionDepartment)
	}

	if strings.TrimSpace(first) == "" || strings.TrimSpace(last) == "" || strings.TrimSpace(dept) == "" {
		return nil, errors.New("missing required workforce fields")
	}

	emailAddr := valueAsString(values, "email", "contactEmail")
	phone := valueAsString(values, "phone", "contactPhone", "contactNumber", "phoneNumber")
	notes := valueAsString(values, "notes", "note", "comment", "message")
	birthday := valueAsString(values, "birthday", "birthDate", "dob")

	req := &models.CreateWorkforceRequest{
		FirstName:  strings.TrimSpace(first),
		LastName:   strings.TrimSpace(last),
		Email:      strings.TrimSpace(emailAddr),
		Phone:      strings.TrimSpace(phone),
		Department: strings.TrimSpace(dept),
	}
	if existing {
		req.Status = models.WorkforceStatusServing
	}
	if notes != "" {
		n := strings.TrimSpace(notes)
		req.Notes = &n
	}
	if birthday != "" {
		b := strings.TrimSpace(birthday)
		req.Birthday = &b
	}
	return req, nil
}

func buildMemberRequest(values map[string]any) (*models.CreateMemberRequest, error) {
	first := valueAsString(values, "firstName", "first_name", "firstname", "givenName")
	last := valueAsString(values, "lastName", "last_name", "lastname", "surname", "familyName")
	if first == "" || last == "" {
		full := valueAsString(values, "fullName", "full_name", "name")
		if full != "" {
			f, l := splitName(full)
			if first == "" {
				first = f
			}
			if last == "" {
				last = l
			}
		}
	}

	emailAddr := valueAsString(values, "email", "contactEmail")
	if strings.TrimSpace(first) == "" || strings.TrimSpace(last) == "" || strings.TrimSpace(emailAddr) == "" {
		return nil, errors.New("missing required member fields")
	}

	phone := valueAsString(values, "phone", "contactPhone", "contactNumber", "phoneNumber")
	birthday := valueAsString(values, "birthday", "birthDate", "dob")

	req := &models.CreateMemberRequest{
		FirstName: strings.TrimSpace(first),
		LastName:  strings.TrimSpace(last),
		Email:     strings.TrimSpace(emailAddr),
	}
	if phone != "" {
		p := strings.TrimSpace(phone)
		req.Phone = &p
	}
	if birthday != "" {
		b := strings.TrimSpace(birthday)
		req.Birthday = &b
	}
	return req, nil
}

func buildLeadershipRequest(values map[string]any) (*models.CreateLeadershipRequest, error) {
	first := valueAsString(values, "firstName", "first_name", "firstname", "givenName")
	last := valueAsString(values, "lastName", "last_name", "lastname", "surname", "familyName")
	if first == "" || last == "" {
		full := valueAsString(values, "fullName", "full_name", "name")
		if full != "" {
			f, l := splitName(full)
			if first == "" {
				first = f
			}
			if last == "" {
				last = l
			}
		}
	}
	if strings.TrimSpace(first) == "" || strings.TrimSpace(last) == "" {
		return nil, errors.New("missing required leadership fields")
	}

	roleRaw := strings.ToLower(strings.TrimSpace(valueAsString(values, "role", "leadershipRole", "leadership_role", "leadershipCategory")))
	role := models.LeadershipRoleAssociatePastor
	switch models.LeadershipRole(roleRaw) {
	case models.LeadershipRoleSeniorPastor, models.LeadershipRoleAssociatePastor, models.LeadershipRoleDeacon, models.LeadershipRoleDeaconess, models.LeadershipRoleReverend:
		role = models.LeadershipRole(roleRaw)
	}

	emailAddr := valueAsString(values, "email", "contactEmail")
	phone := valueAsString(values, "phone", "contactPhone", "contactNumber", "phoneNumber")
	bio := valueAsString(values, "bio", "notes", "note", "comment", "message", "about")
	imageURL := valueAsString(values, "imageUrl", "image", "profileImage", "profile_image")
	birthday := valueAsString(values, "birthday", "birthDate", "dob")
	anniversary := valueAsString(values, "anniversary", "weddingAnniversary", "anniversaryDate")

	req := &models.CreateLeadershipRequest{
		FirstName: strings.TrimSpace(first),
		LastName:  strings.TrimSpace(last),
		Email:     strings.TrimSpace(emailAddr),
		Phone:     strings.TrimSpace(phone),
		Role:      role,
	}
	if strings.TrimSpace(bio) != "" {
		clean := strings.TrimSpace(bio)
		req.Bio = &clean
	}
	if strings.TrimSpace(imageURL) != "" {
		clean := strings.TrimSpace(imageURL)
		req.ImageURL = &clean
	}
	if strings.TrimSpace(birthday) != "" {
		clean := strings.TrimSpace(birthday)
		req.Birthday = &clean
	}
	if strings.TrimSpace(anniversary) != "" {
		clean := strings.TrimSpace(anniversary)
		req.Anniversary = &clean
	}

	return req, nil
}

func buildTestimonialRequest(values map[string]any) (*models.CreateTestimonialRequest, error) {
	first := valueAsString(values, "firstName", "first_name", "firstname", "givenName")
	last := valueAsString(values, "lastName", "last_name", "lastname", "surname", "familyName")
	if first == "" || last == "" {
		full := valueAsString(values, "fullName", "full_name", "name")
		if full != "" {
			f, l := splitName(full)
			if first == "" {
				first = f
			}
			if last == "" {
				last = l
			}
		}
	}
	if strings.TrimSpace(first) == "" || strings.TrimSpace(last) == "" {
		return nil, errors.New("missing required testimonial fields")
	}

	testimony := strings.TrimSpace(valueAsString(values, "testimony", "testimonyText", "message", "content", "story", "note", "notes"))
	if testimony == "" {
		return nil, errors.New("missing required testimonial fields")
	}

	imageURL := strings.TrimSpace(valueAsString(values, "imageUrl", "image", "profileImage", "profile_image", "photo"))
	isAnonymousRaw := strings.ToLower(strings.TrimSpace(valueAsString(values, "isAnonymous", "anonymous")))
	isAnonymous := isAnonymousRaw == "true" || isAnonymousRaw == "1" || isAnonymousRaw == "yes"

	req := &models.CreateTestimonialRequest{
		FirstName:   strings.TrimSpace(first),
		LastName:    strings.TrimSpace(last),
		Testimony:   testimony,
		IsAnonymous: isAnonymous,
	}
	if imageURL != "" {
		req.ImageURL = &imageURL
	}

	return req, nil
}

func valueAsString(values map[string]any, keys ...string) string {
	if values == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := values[k]; ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return t
				}
			default:
				s := strings.TrimSpace(fmt.Sprint(t))
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func splitName(full string) (string, string) {
	parts := strings.Fields(full)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
