// internal/service/form_service.go
//
// The FormService implementation is split across several files by concern —
// this one holds the interface, constructor, and core CRUD. See also:
// form_service_public.go (public-facing reads/submission), form_service_settings.go
// (form settings encode/decode), form_service_fields.go (field validation rules),
// form_service_validation.go (submission validation), form_service_registration.go
// (registration codes/response email), form_service_campaign.go (campaign email),
// form_service_calendar.go (calendar reminders/ICS), form_service_target_sync.go
// (syncing submissions to workforce/member/leadership/testimonial records).
package service

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

var slugInvalidRe = regexp.MustCompile(`[^a-z0-9\-]+`)
var slugDashCollapseRe = regexp.MustCompile(`-+`)
var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var phoneRe = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)
var ddDashRe = regexp.MustCompile(`^(\d{1,2})-(\d{1,2})(?:-(\d{2,4}))?$`)
var ddSlashRe = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})(?:/(\d{2,4}))?$`)
var templateKeyRe = regexp.MustCompile(`^[A-Za-z0-9/_-]+$`)
var dataImageRe = regexp.MustCompile(`^data:image\/(?:png|jpe?g|webp);base64,`)
var ErrFormExpired = errors.New("form expired")
var ErrFormClosed = errors.New("registration closed")
var ErrFormReportAccessDenied = errors.New("invalid report link")

const canonicalLeadershipFormSlug = "leadership-biodata"

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
	DeleteSubmission(id string) error
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
	uploader       AssetUploader

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
	uploader AssetUploader,
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
		uploader:        uploader,
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
	if limit < 1 || limit > 300 {
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
				if isCanonicalLeadershipFormRequest(req, slug) {
					existing, err := s.repo.GetAnyBySlug(slug)
					if err != nil {
						return nil, err
					}
					s.attachPublicURL(existing)
					return existing, nil
				}
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

func isCanonicalLeadershipFormRequest(req *models.CreateFormRequest, slug string) bool {
	if req == nil || slug != canonicalLeadershipFormSlug || req.Settings == nil {
		return false
	}

	formType := ""
	if req.Settings.FormType != nil {
		formType = strings.ToLower(strings.TrimSpace(*req.Settings.FormType))
	}

	submissionTarget := ""
	if req.Settings.SubmissionTarget != nil {
		submissionTarget = strings.ToLower(strings.TrimSpace(*req.Settings.SubmissionTarget))
	}

	return formType == "leadership" && submissionTarget == "leadership"
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
		// Updates should stay editable for draft-style builder iterations.
		// Strict publish checks still run in Publish().
		fields, err := buildAndValidateFields(existing.ID, *req.Fields, true)
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
