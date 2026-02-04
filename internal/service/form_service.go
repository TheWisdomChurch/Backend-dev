// internal/service/form_service.go
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

var slugInvalidRe = regexp.MustCompile(`[^a-z0-9\-]+`)
var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var phoneRe = regexp.MustCompile(`^[0-9()+\-\s]{7,20}$`)
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
	Publish(id string) (string, error)

	GetPublic(slug string) (*models.PublicFormPayload, error)
	Submit(slug string, req *models.SubmitFormRequest) error

	// Admin submissions
	ListSubmissions(formID string, page, limit int, start, end *time.Time) ([]models.FormSubmission, int64, error)
	Stats(start, end *time.Time) (*models.FormStatsResponse, error)
	CleanupExpiredForms(now time.Time) (int64, error)
}

type formService struct {
	repo repository.FormRepository

	// IMPORTANT:
	// In your codebase EventRepository is a concrete type (pointer), not an interface.
	// So keep it as a pointer and nil-check works.
	eventRepo *repository.EventRepository
}

func NewFormService(repo repository.FormRepository, eventRepo *repository.EventRepository) FormService {
	return &formService{repo: repo, eventRepo: eventRepo}
}

func (s *formService) List(page, limit int) ([]models.Form, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit
	return s.repo.List(offset, limit)
}

func (s *formService) GetByID(id string) (*models.Form, error) {
	return s.repo.GetByID(id)
}

func (s *formService) Create(req *models.CreateFormRequest) (*models.Form, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.New("title is required")
	}
	title := strings.TrimSpace(req.Title)
	exists, err := s.repo.TitleExists(title, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("a form with this title already exists")
	}

	settingsJSON, err := encodeSettings(req.Settings)
	if err != nil {
		return nil, err
	}

	form := &models.Form{
		Title:       title,
		Description: req.Description,
		EventID:     req.EventID,
		IsPublished: false,
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

	return s.repo.GetByID(form.ID)
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
		exists, err := s.repo.TitleExists(t, existing.ID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("a form with this title already exists")
		}
		existing.Title = t
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.EventID != nil {
		existing.EventID = req.EventID
	}
	if req.Settings != nil {
		settingsJSON, err := encodeSettings(req.Settings)
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

	return s.repo.GetByID(existing.ID)
}

func (s *formService) Delete(id string) error {
	return s.repo.Delete(id)
}

func (s *formService) Publish(id string) (string, error) {
	form, err := s.repo.GetByID(id)
	if err != nil {
		return "", err
	}

	if len(form.Fields) == 0 {
		return "", errors.New("cannot publish: add at least one field")
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
		return "", err
	}

	base := slugify(form.Title)
	if base == "" {
		base = "form"
	}

	slug := base
	i := 2
	for {
		exists, err := s.repo.SlugExists(slug)
		if err != nil {
			return "", err
		}
		if !exists {
			break
		}
		slug = fmt.Sprintf("%s-%d", base, i)
		i++
	}

	form.IsPublished = true
	form.Slug = &slug

	if err := s.repo.Update(form); err != nil {
		return "", err
	}

	return slug, nil
}

func (s *formService) GetPublic(slug string) (*models.PublicFormPayload, error) {
	form, err := s.repo.GetBySlug(slug)
	if err != nil {
		return nil, err
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
			return errors.New("form expired")
		}
	}

	if settings.ClosesAt != nil && strings.TrimSpace(*settings.ClosesAt) != "" {
		t, parseErr := parseFlexibleTime(*settings.ClosesAt)
		if parseErr != nil {
			return errors.New("form closesAt is invalid on server")
		}
		if time.Now().After(t) {
			return errors.New("registration closed")
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

	valuesJSON, err := json.Marshal(cleanValues)
	if err != nil {
		return errors.New("failed to store submission")
	}

	// Extract common fields into columns for analytics
	name, email, phone, addr := extractCommonFields(cleanValues)

	sub := &models.FormSubmission{
		FormID:         form.ID,
		Name:           name,
		Email:          email,
		ContactNumber:  phone,
		ContactAddress: addr,
		Values:         datatypes.JSON(valuesJSON),
	}

	return s.repo.CreateSubmission(sub)
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

		clean[f.Key] = v

		switch f.Type {
		case models.FieldCheckbox:
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be boolean", f.Key)
			}
			if f.Required && !b {
				return nil, fmt.Errorf("field '%s' must be accepted", f.Key)
			}

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

func valueToString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
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
func extractCommonFields(values map[string]any) (*string, *string, *string, *string) {
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

	name := lookup("fullName", "name", "full_name")
	email := lookup("email", "contactEmail")
	phone := lookup("phone", "contactPhone", "contactNumber", "phoneNumber")
	addr := lookup("address", "contactAddress")

	return name, email, phone, addr
}
