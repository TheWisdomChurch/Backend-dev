// internal/service/form_service.go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

var slugInvalidRe = regexp.MustCompile(`[^a-z0-9\-]+`)
var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var phoneRe = regexp.MustCompile(`^[0-9()+\-\s]{7,20}$`)
var templateKeyRe = regexp.MustCompile(`^[A-Za-z0-9/_-]+$`)
var ErrFormExpired = errors.New("form expired")
var ErrFormClosed = errors.New("registration closed")
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

	GetPublic(slug string) (*models.PublicFormPayload, error)
	Submit(slug string, req *models.SubmitFormRequest) error

	// Admin submissions
	ListSubmissions(formID string, page, limit int, start, end *time.Time) ([]models.FormSubmission, int64, error)
	Stats(start, end *time.Time) (*models.FormStatsResponse, error)
	StatsByForm(formID string, start, end *time.Time) ([]models.FormSubmissionDailyCount, error)
	CleanupExpiredForms(now time.Time) (int64, error)
}

type formService struct {
	repo repository.FormRepository

	// IMPORTANT:
	// In your codebase EventRepository is a concrete type (pointer), not an interface.
	// So keep it as a pointer and nil-check works.
	eventRepo *repository.EventRepository

	sequenceRepo *repository.RegistrationSequenceRepository
	templateRepo repository.EmailTemplateRepository
	workforceSvc WorkforceService
	memberSvc    MemberService
	sender       EmailSender
	branding     email.Branding

	publicBaseURL   string
	tplStore        *email.TemplateStore
	templateTimeout time.Duration
}

func NewFormService(
	repo repository.FormRepository,
	eventRepo *repository.EventRepository,
	sequenceRepo *repository.RegistrationSequenceRepository,
	templateRepo repository.EmailTemplateRepository,
	workforceSvc WorkforceService,
	memberSvc MemberService,
	sender EmailSender,
	branding email.Branding,
	publicBaseURL string,
) FormService {
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

	return &formService{
		repo:            repo,
		eventRepo:       eventRepo,
		sequenceRepo:    sequenceRepo,
		templateRepo:    templateRepo,
		workforceSvc:    workforceSvc,
		memberSvc:       memberSvc,
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

func (s *formService) GetPublic(slug string) (*models.PublicFormPayload, error) {
	form, err := s.repo.GetBySlug(slug)
	if err != nil {
		return nil, err
	}

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

	cleanValues, err := validateSubmission(form.Fields, req.Values)
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
	name, email, phone, addr := extractCommonFields(form.Fields, cleanValues)

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

	if regCode != nil && email != nil && s.sender != nil {
		s.sendRegistrationCodeEmail(form, *email, name, *regCode)
	}
	if email != nil {
		s.sendResponseEmail(form, settings, cleanValues, name, *email, regCode)
	}
	if err := s.syncSubmissionTarget(form, settings, cleanValues); err != nil {
		log.Printf("⚠️ submission target sync failed: %v", err)
	}

	return nil
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
	if s.ResponseEmailTemplateKey != nil {
		key := strings.Trim(strings.TrimSpace(*s.ResponseEmailTemplateKey), "/")
		if key == "" {
			s.ResponseEmailTemplateKey = nil
		} else {
			if strings.Contains(key, "..") || !templateKeyRe.MatchString(key) {
				return nil, fmt.Errorf("responseEmailTemplateKey contains invalid characters")
			}
			s.ResponseEmailTemplateKey = &key
		}
	}
	if s.SubmissionTarget != nil {
		target := strings.ToLower(strings.TrimSpace(*s.SubmissionTarget))
		switch target {
		case "", "none":
			s.SubmissionTarget = nil
		case "workforce", "workforce_new", "workforce_serving", "member":
			s.SubmissionTarget = &target
		default:
			return nil, fmt.Errorf("submissionTarget must be workforce, workforce_new, workforce_serving, or member")
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
	prefix := fmt.Sprintf("REG-%s", initials)
	seq, err := s.sequenceRepo.Next(prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%04d", prefix, seq), nil
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

func (s *formService) sendResponseEmail(form *models.Form, settings *models.FormSettingsDTO, values map[string]any, name *string, emailAddr string, regCode *string) {
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
	if strings.TrimSpace(s.branding.PublicURL) != "" {
		subscribeURL = strings.TrimRight(s.branding.PublicURL, "/") + "/api/v1/notifications/subscribe?email=" + url.QueryEscape(addr)
		if recipient != "" {
			subscribeURL += "&name=" + url.QueryEscape(recipient)
		}
	}

	code := ""
	if regCode != nil {
		code = strings.TrimSpace(*regCode)
	}

	hero := ""
	if settings != nil && settings.Design != nil && settings.Design.CoverImageURL != nil {
		hero = strings.TrimSpace(*settings.Design.CoverImageURL)
	}
	if hero == "" && event.BannerImage != nil {
		hero = strings.TrimSpace(*event.BannerImage)
	}
	if hero == "" && event.Image != nil {
		hero = strings.TrimSpace(*event.Image)
	}

	now := time.Now().UTC()
	templateData := map[string]any{
		"Branding":         s.branding,
		"Form":             form,
		"Event":            event,
		"Values":           values,
		"RecipientName":    recipient,
		"FullName":         recipient,
		"Name":             recipient,
		"FirstName":        firstToken(recipient),
		"Email":            addr,
		"RegistrationCode": code,
		"FormURL":          formURL,
		"PublicURL":        formURL,
		"SubscribeURL":     subscribeURL,
		"FormTitle":        formTitle,
		"EventTitle":       strings.TrimSpace(event.Title),
		"EventDate":        strings.TrimSpace(event.Date),
		"EventTime":        strings.TrimSpace(event.Time),
		"EventLocation":    strings.TrimSpace(event.Location),
		"HeroImageURL":     hero,
		"SubmittedAt":      now.Format(time.RFC3339),
		"SubmittedAtText":  now.Format("Mon, 02 Jan 2006 15:04 MST"),
		"Year":             now.Year(),
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
			}
		}
	}

	if strings.TrimSpace(body) == "" && templateKey != "" && s.tplStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.templateTimeout)
		defer cancel()

		_, htmlOut, _, err := s.tplStore.RenderWithData(ctx, templateKey, templateData)
		if err == nil && strings.TrimSpace(htmlOut) != "" {
			body = htmlOut
		}
	}

	if strings.TrimSpace(body) == "" {
		message := ""
		if settings != nil && settings.SuccessMessage != nil {
			message = strings.TrimSpace(*settings.SuccessMessage)
		}

		body = email.RenderFormResponseEmail(email.FormResponseTemplateData{
			Branding:         s.branding,
			RecipientName:    recipient,
			FormTitle:        formTitle,
			EventTitle:       strings.TrimSpace(event.Title),
			EventDate:        strings.TrimSpace(event.Date),
			EventTime:        strings.TrimSpace(event.Time),
			EventLocation:    strings.TrimSpace(event.Location),
			RegistrationCode: code,
			Message:          message,
			FormURL:          formURL,
			HeroImageURL:     hero,
		})
	}

	if err := s.sender.SendHTML(addr, subject, body); err != nil {
		log.Printf("⚠️ failed to send form response email to %s (templateKey=%s, formID=%s): %v", addr, templateKey, formID, err)
	}
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
