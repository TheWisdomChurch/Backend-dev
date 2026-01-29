// internal/service/form_service.go
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

var slugInvalidRe = regexp.MustCompile(`[^a-z0-9\-]+`)
var hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

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

	settingsJSON, err := encodeSettings(req.Settings)
	if err != nil {
		return nil, err
	}

	form := &models.Form{
		Title:       strings.TrimSpace(req.Title),
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
		t, parseErr := time.Parse(time.RFC3339, *settings.ExpiresAt)
		if parseErr != nil {
			return errors.New("form expiresAt is invalid on server")
		}
		if time.Now().After(t) {
			return errors.New("form expired")
		}
	}

	if settings.ClosesAt != nil && strings.TrimSpace(*settings.ClosesAt) != "" {
		t, parseErr := time.Parse(time.RFC3339, *settings.ClosesAt)
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

	if err := validateSubmission(form.Fields, req.Values); err != nil {
		return err
	}

	valuesJSON, err := json.Marshal(req.Values)
	if err != nil {
		return errors.New("failed to store submission")
	}

	// Extract common fields into columns for analytics
	name, email, phone, addr := extractCommonFields(req.Values)

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
	return s.repo.DeleteExpired(now)
}

/* =========================
   Helpers: settings/options
========================= */

func encodeSettings(s *models.FormSettingsDTO) (datatypes.JSON, error) {
	if s == nil {
		return datatypes.JSON([]byte("null")), nil
	}
	if s.Capacity != nil && *s.Capacity < 0 {
		return nil, errors.New("capacity cannot be negative")
	}
	if s.ClosesAt != nil && strings.TrimSpace(*s.ClosesAt) != "" {
		if _, err := time.Parse(time.RFC3339, *s.ClosesAt); err != nil {
			return nil, errors.New("closesAt must be RFC3339 ISO string")
		}
	}
	if s.ExpiresAt != nil && strings.TrimSpace(*s.ExpiresAt) != "" {
		if _, err := time.Parse(time.RFC3339, *s.ExpiresAt); err != nil {
			return nil, errors.New("expiresAt must be RFC3339 ISO string")
		}
	}

	// Normalize and validate optional design settings
	if s.Design != nil {
		d := s.Design

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

		if layout := d.Layout; layout != nil {
			lv := strings.ToLower(strings.TrimSpace(*layout))
			if lv == "" {
				d.Layout = nil
			} else if lv != "split" && lv != "stacked" && lv != "inline" {
				return nil, fmt.Errorf("design.layout must be one of: split, stacked, inline")
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
		if d.PrimaryColor, err = normalizeColor("design.primaryColor", d.PrimaryColor); err != nil {
			return nil, err
		}
		if d.BackgroundColor, err = normalizeColor("design.backgroundColor", d.BackgroundColor); err != nil {
			return nil, err
		}
		if d.AccentColor, err = normalizeColor("design.accentColor", d.AccentColor); err != nil {
			return nil, err
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

	seenKey := map[string]bool{}
	out := make([]models.FormField, 0, len(fields))

	for i, f := range fields {
		key := strings.TrimSpace(f.Key)
		label := strings.TrimSpace(f.Label)
		typ := strings.TrimSpace(f.Type)

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

		var optionsJSON datatypes.JSON
		if typ == string(models.FieldSelect) || typ == string(models.FieldRadio) {
			if len(f.Options) == 0 {
				return nil, fmt.Errorf("field[%d]: options required for type '%s'", i, typ)
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

		out = append(out, models.FormField{
			FormID:   formID,
			Key:      key,
			Label:    label,
			Type:     models.FormFieldType(typ),
			Required: f.Required,
			Options:  optionsJSON,
			Order:    f.Order,
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

/* =========================
   Submission validation
========================= */

func validateSubmission(fields []models.FormField, values map[string]any) error {
	fieldByKey := map[string]models.FormField{}
	for _, f := range fields {
		fieldByKey[f.Key] = f
	}

	for k := range values {
		if _, ok := fieldByKey[k]; !ok {
			return fmt.Errorf("unknown field '%s'", k)
		}
	}

	for _, f := range fields {
		v, exists := values[f.Key]

		if f.Required {
			if !exists || v == nil {
				return fmt.Errorf("field '%s' is required", f.Key)
			}
			if f.Type == models.FieldCheckbox {
				b, ok := v.(bool)
				if !ok || !b {
					return fmt.Errorf("field '%s' must be accepted", f.Key)
				}
			} else {
				sv, ok := v.(string)
				if !ok || strings.TrimSpace(sv) == "" {
					return fmt.Errorf("field '%s' is required", f.Key)
				}
			}
		}

		if !exists || v == nil {
			continue
		}

		switch f.Type {
		case models.FieldCheckbox:
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("field '%s' must be boolean", f.Key)
			}
		default:
			sv, ok := v.(string)
			if !ok {
				return fmt.Errorf("field '%s' must be string", f.Key)
			}

			if f.Type == models.FieldSelect || f.Type == models.FieldRadio {
				opts := decodeOptionsToDTO(f.Options)
				allowed := map[string]bool{}
				for _, o := range opts {
					allowed[o.Value] = true
				}
				if !allowed[sv] {
					return fmt.Errorf("field '%s' has invalid option", f.Key)
				}
			}
		}
	}

	return nil
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
