// internal/service/form_service_settings.go
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
)

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

func normalizeLegacyFormContentSections(sections *[]models.FormLegacyContentSectionDTO) error {
	if sections == nil {
		return nil
	}

	cleanSections := make([]models.FormLegacyContentSectionDTO, 0, len(*sections))
	for i, section := range *sections {
		var normalized models.FormLegacyContentSectionDTO

		if section.Title != nil {
			title := strings.TrimSpace(*section.Title)
			if title != "" {
				if utf8.RuneCountInString(title) > 160 {
					return fmt.Errorf("contentSections[%d].title too long", i)
				}
				normalized.Title = &title
			}
		}
		if section.Subtitle != nil {
			subtitle := strings.TrimSpace(*section.Subtitle)
			if subtitle != "" {
				if utf8.RuneCountInString(subtitle) > 260 {
					return fmt.Errorf("contentSections[%d].subtitle too long", i)
				}
				normalized.Subtitle = &subtitle
			}
		}

		items := make([]string, 0, len(section.Items))
		for j, item := range section.Items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if utf8.RuneCountInString(item) > 160 {
				return fmt.Errorf("contentSections[%d].items[%d] too long", i, j)
			}
			items = append(items, item)
		}
		normalized.Items = items

		itemSubtexts := make([]string, 0, len(section.ItemSubtexts))
		for j, itemSubtext := range section.ItemSubtexts {
			itemSubtext = strings.TrimSpace(itemSubtext)
			if itemSubtext == "" {
				continue
			}
			if utf8.RuneCountInString(itemSubtext) > 260 {
				return fmt.Errorf("contentSections[%d].itemSubtexts[%d] too long", i, j)
			}
			itemSubtexts = append(itemSubtexts, itemSubtext)
		}
		normalized.ItemSubtexts = itemSubtexts

		if normalized.Title == nil && normalized.Subtitle == nil && len(normalized.Items) == 0 && len(normalized.ItemSubtexts) == 0 {
			continue
		}

		cleanSections = append(cleanSections, normalized)
	}

	*sections = cleanSections
	return nil
}

func convertLegacyToExtendedSections(sections []models.FormLegacyContentSectionDTO) []models.FormContentSectionDTO {
	converted := make([]models.FormContentSectionDTO, 0, len(sections))
	for idx, legacy := range sections {
		section := models.FormContentSectionDTO{
			Items: make([]models.FormContentSectionItemDTO, 0),
		}
		if legacy.Title != nil {
			section.Title = strings.TrimSpace(*legacy.Title)
		}
		if legacy.Subtitle != nil {
			subtitle := strings.TrimSpace(*legacy.Subtitle)
			if subtitle != "" {
				section.Subtitle = &subtitle
			}
		}
		for i, itemTitle := range legacy.Items {
			itemTitle = strings.TrimSpace(itemTitle)
			if itemTitle == "" {
				continue
			}
			item := models.FormContentSectionItemDTO{Title: itemTitle}
			if i < len(legacy.ItemSubtexts) {
				subtext := strings.TrimSpace(legacy.ItemSubtexts[i])
				if subtext != "" {
					itemBody := subtext
					item.Body = &itemBody
				}
			}
			section.Items = append(section.Items, item)
		}
		if strings.TrimSpace(section.Title) == "" {
			if len(section.Items) > 0 {
				section.Title = section.Items[0].Title
			} else {
				section.Title = fmt.Sprintf("Section %d", idx+1)
			}
		}
		converted = append(converted, section)
	}
	return converted
}

func convertExtendedToLegacySections(sections []models.FormContentSectionDTO) []models.FormLegacyContentSectionDTO {
	converted := make([]models.FormLegacyContentSectionDTO, 0, len(sections))
	for _, section := range sections {
		legacy := models.FormLegacyContentSectionDTO{
			Items:        make([]string, 0, len(section.Items)),
			ItemSubtexts: make([]string, 0, len(section.Items)),
		}
		if title := strings.TrimSpace(section.Title); title != "" {
			legacy.Title = &title
		}
		if section.Subtitle != nil {
			subtitle := strings.TrimSpace(*section.Subtitle)
			if subtitle != "" {
				legacy.Subtitle = &subtitle
			}
		}

		for _, item := range section.Items {
			itemTitle := strings.TrimSpace(item.Title)
			if itemTitle == "" {
				continue
			}
			legacy.Items = append(legacy.Items, itemTitle)

			itemSubtext := ""
			if item.Body != nil {
				itemSubtext = strings.TrimSpace(*item.Body)
			}
			legacy.ItemSubtexts = append(legacy.ItemSubtexts, itemSubtext)
		}
		converted = append(converted, legacy)
	}
	return converted
}

func encodeSettings(s *models.FormSettingsDTO) (datatypes.JSON, error) {
	if s == nil {
		return datatypes.JSON([]byte("null")), nil
	}
	if s.Capacity != nil && *s.Capacity < 0 {
		return nil, errors.New("capacity cannot be negative")
	}
	if s.ResponseEmailEnabled == nil {
		defaultResponseEmail := true
		s.ResponseEmailEnabled = &defaultResponseEmail
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
	if s.SuccessTitle, err = normalizeText("successTitle", s.SuccessTitle, 200); err != nil {
		return nil, err
	}
	if s.SuccessSubtitle, err = normalizeText("successSubtitle", s.SuccessSubtitle, 260); err != nil {
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
		case "leadership-form", "leadership_form", "leadership application", "leadership_application":
			normalized := "leadership"
			s.SubmissionTarget = &normalized
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
		case "leadership-form", "leadership_form", "leadership application", "leadership_application":
			formType = "leadership"
		case "workforce-form", "workforce_form", "workforce application", "workforce_application":
			formType = "workforce"
		case "member-form", "member_form", "membership form":
			formType = "membership"
		}
		switch formType {
		case "":
			s.FormType = nil
		case "registration", "event", "membership", "workforce", "leadership", "testimonial", "application", "contact", "general":
			s.FormType = &formType
		default:
			return nil, fmt.Errorf("formType must be one of: registration, event, membership, workforce, leadership, testimonial, application, contact, general")
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
		case "yyyy-mm-dd", "mm/dd/yyyy", "dd/mm/yyyy", "dd/mm", "dd-mm":
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
	if err := normalizeLegacyFormContentSections(s.ContentSections); err != nil {
		return nil, err
	}

	if s.Sections == nil && s.ContentSections != nil {
		converted := convertLegacyToExtendedSections(*s.ContentSections)
		if err := normalizeFormContentSections(&converted); err != nil {
			return nil, err
		}
		s.Sections = &converted
	}
	if s.ContentSections == nil && s.Sections != nil {
		legacy := convertExtendedToLegacySections(*s.Sections)
		if err := normalizeLegacyFormContentSections(&legacy); err != nil {
			return nil, err
		}
		s.ContentSections = &legacy
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
			case "yyyy-mm-dd", "mm/dd/yyyy", "dd/mm/yyyy", "dd/mm", "dd-mm":
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
	if s.Sections != nil && s.ContentSections == nil {
		legacy := convertExtendedToLegacySections(*s.Sections)
		s.ContentSections = &legacy
	}
	if s.ContentSections != nil && s.Sections == nil {
		converted := convertLegacyToExtendedSections(*s.ContentSections)
		s.Sections = &converted
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
