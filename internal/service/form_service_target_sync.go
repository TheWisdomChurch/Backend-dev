// internal/service/form_service_target_sync.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/datatypes"

	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
)

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

func resolveSubmissionTarget(settings *models.FormSettingsDTO) string {
	if settings == nil {
		return ""
	}
	if settings.SubmissionTarget != nil {
		if target := strings.ToLower(strings.TrimSpace(*settings.SubmissionTarget)); target != "" {
			return target
		}
	}
	if settings.FormType != nil {
		switch strings.ToLower(strings.TrimSpace(*settings.FormType)) {
		case "testimonial", "member", "leadership":
			return strings.ToLower(strings.TrimSpace(*settings.FormType))
		case "workforce":
			return "workforce"
		}
	}
	return ""
}

func resolveSubmissionTargetForForm(form *models.Form, settings *models.FormSettingsDTO) string {
	if target := resolveSubmissionTarget(settings); target != "" {
		return target
	}
	if form == nil {
		return ""
	}

	slug := strings.ToLower(strings.TrimSpace(valueOrEmpty(form.Slug)))
	title := strings.ToLower(strings.TrimSpace(form.Title))
	surface := strings.TrimSpace(slug + " " + title)
	if surface == "" {
		return ""
	}

	if strings.Contains(surface, "testimony") || strings.Contains(surface, "testimonial") {
		return "testimonial"
	}
	if strings.Contains(surface, "leadership") {
		return "leadership"
	}
	if strings.Contains(surface, "membership") || strings.Contains(surface, "member") {
		return "member"
	}
	if strings.Contains(surface, "workforce") || strings.Contains(surface, "worker") {
		return "workforce"
	}
	return ""
}

// notifySubmissionTargetSyncFailure surfaces a submission-routing failure to
// super admins. The submission itself is never lost — it's already durably
// saved in form_submissions — but without this, a routing failure (e.g. a
// form's field names don't match what the target (workforce/member/leadership)
// expects) was previously visible only in a warn-level log line nobody reads,
// silently leaving the record out of the target table with no indication why.
func (s *formService) notifySubmissionTargetSyncFailure(form *models.Form, submissionID, target string, syncErr error) {
	if s.notifySvc == nil || form == nil {
		return
	}
	title := strings.TrimSpace(form.Title)
	if title == "" {
		title = "Untitled form"
	}
	message := fmt.Sprintf(
		"A submission to %q could not be routed to %s automatically (%s). The raw submission is saved — open the form's submissions to review and add it manually.",
		title, target, syncErr.Error(),
	)
	entityType := "form_submission"
	entityID := submissionID
	_ = s.notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "submission_sync_failed",
		Title:      "Form submission needs manual review",
		Message:    message,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"admin", "super_admin"},
	})
}

func (s *formService) syncSubmissionTarget(form *models.Form, settings *models.FormSettingsDTO, values map[string]any, submissionID string) error {
	target := resolveSubmissionTargetForForm(form, settings)
	if target == "" {
		return nil
	}
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
		created, err := s.workforceSvc.CreateApplication(req)
		if err != nil {
			return err
		}
		if created != nil {
			s.syncWeddingAnniversary(string(models.WeddingAnniversarySubjectWorkforce), created.ID, values, submissionID)
		}
		return nil
	case "workforce_serving":
		if s.workforceSvc == nil {
			return errors.New("workforce service not configured")
		}
		req, err := buildWorkforceRequest(values, settings, true)
		if err != nil {
			return err
		}
		created, err := s.workforceSvc.RegisterExisting(req)
		if err != nil {
			return err
		}
		if created != nil {
			s.syncWeddingAnniversary(string(models.WeddingAnniversarySubjectWorkforce), created.ID, values, submissionID)
		}
		return nil
	case "member":
		if s.memberSvc == nil {
			return errors.New("member service not configured")
		}
		req, err := buildMemberRequest(values)
		if err != nil {
			return err
		}
		created, err := s.memberSvc.Create(req)
		if err != nil {
			return err
		}
		if created != nil {
			s.syncWeddingAnniversary(string(models.WeddingAnniversarySubjectMember), created.ID, values, submissionID)
		}
		return nil
	case "leadership":
		if s.leadershipSvc == nil {
			return errors.New("leadership service not configured")
		}
		req, err := buildLeadershipRequest(values)
		if err != nil {
			return err
		}
		created, err := s.leadershipSvc.Apply(req)
		if err != nil {
			return err
		}
		if created != nil {
			s.syncWeddingAnniversary(string(models.WeddingAnniversarySubjectLeadership), created.ID, values, submissionID)
		}
		return nil
	case "testimonial":
		if s.testimonialSvc == nil {
			return errors.New("testimonial service not configured")
		}
		req, err := buildTestimonialRequest(values)
		if err != nil {
			req = buildLenientTestimonialRequest(values)
			if req == nil {
				return err
			}
		}
		_, err = s.testimonialSvc.CreateTestimonial(req)
		return err
	default:
		return nil
	}
}

// BuildWorkforceRequestFromValues exposes the same field-mapping logic
// syncSubmissionTarget uses at submission time, for one-off maintenance
// scripts (e.g. backfilling birthday/anniversary from a submission whose
// original mapping predates a fix) that need it without duplicating the
// key-lookup list.
func BuildWorkforceRequestFromValues(values map[string]any) (*models.CreateWorkforceRequest, error) {
	return buildWorkforceRequest(values, nil, false)
}

// buildWeddingAnniversaryInput pulls the wedding-anniversary + spouse answers
// out of a submission's values map. ok is false when no anniversary date was
// supplied (the common case for forms that don't ask). Recognised field keys:
//
//	anniversary date  : weddingAnniversary | wedding_anniversary | anniversary | anniversaryDate
//	spouse name        : spouseName | spouse_name | spouseFullName | partnerName | spouse
//	spouse email       : spouseEmail | spouse_email | partnerEmail
//	consent to email   : spouseEmailConsent | spouseConsent | partnerConsentToEmail
func buildWeddingAnniversaryInput(values map[string]any) (models.WeddingAnniversaryInput, bool) {
	date := strings.TrimSpace(valueAsString(values,
		"weddingAnniversary", "wedding_anniversary", "anniversary", "anniversaryDate", "anniversary_date",
	))
	if date == "" {
		return models.WeddingAnniversaryInput{}, false
	}
	in := models.WeddingAnniversaryInput{Anniversary: &date}

	in.SpouseName = strings.TrimSpace(valueAsString(values,
		"spouseName", "spouse_name", "spouseFullName", "spouse_full_name", "partnerName", "partner_name", "spouse",
	))
	if email := strings.TrimSpace(valueAsString(values, "spouseEmail", "spouse_email", "partnerEmail", "partner_email")); email != "" {
		in.SpouseEmail = &email
		in.SpouseEmailConsent = parseFormBool(valueAsString(values,
			"spouseEmailConsent", "spouse_email_consent", "spouseConsent", "spouse_consent", "partnerConsentToEmail", "partner_consent_to_email",
		))
	}
	if raw := strings.TrimSpace(valueAsString(values, "spouseIsExternal", "spouse_is_external", "spouseAttendsAnotherChurch", "spouse_attends_another_church")); raw != "" {
		ext := parseFormBool(raw)
		in.SpouseIsExternal = &ext
	}
	return in, true
}

func parseFormBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "y", "on", "checked":
		return true
	default:
		return false
	}
}

// syncWeddingAnniversary upserts the marriage record for a person just created
// from a form submission. Best-effort: a failure here never fails the
// submission (the raw answers are already saved), it only surfaces to admins.
func (s *formService) syncWeddingAnniversary(subjectType, subjectID string, values map[string]any, submissionID string) {
	if s.weddingSvc == nil || strings.TrimSpace(subjectID) == "" {
		return
	}
	in, ok := buildWeddingAnniversaryInput(values)
	if !ok {
		return
	}
	var subPtr *string
	if strings.TrimSpace(submissionID) != "" {
		v := strings.TrimSpace(submissionID)
		subPtr = &v
	}
	if _, err := s.weddingSvc.UpsertForSubject(context.Background(), subjectType, subjectID, in, models.WeddingAnniversarySourceForm, subPtr); err != nil {
		applog.L().Warn("wedding anniversary sync from form failed",
			"subject_type", subjectType, "subject_id", subjectID, "error", err)
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

	emailAddr := valueAsString(values, "email", "contactEmail", "emailAddress", "email_address")
	phone := valueAsString(values, "phone", "contactPhone", "contactNumber", "phoneNumber", "contact_number")
	notes := valueAsString(values, "notes", "note", "comment", "message")
	birthday := valueAsString(values, "birthday", "birthDate", "birth_date", "dob", "dateOfBirth", "date_of_birth")
	anniversary := valueAsString(values, "anniversary", "weddingAnniversary", "wedding_anniversary", "anniversaryDate", "anniversary_date")

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
	if strings.TrimSpace(birthday) != "" {
		b := strings.TrimSpace(birthday)
		req.Birthday = &b
	}
	if strings.TrimSpace(anniversary) != "" {
		a := strings.TrimSpace(anniversary)
		req.Anniversary = &a
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

	emailAddr := valueAsString(values, "email", "contactEmail", "emailAddress", "email_address")
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
	// Form-created member records remain inactive until reviewed in admin.
	isActive := false
	req.IsActive = &isActive
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

func isDataImageValue(value string) bool {
	return dataImageRe.MatchString(strings.ToLower(strings.TrimSpace(value)))
}

func buildLeadershipRequest(values map[string]any) (*models.CreateLeadershipRequest, error) {
	first := valueAsString(values, "firstName", "first_name", "firstname", "givenName")
	last := valueAsString(values, "lastName", "last_name", "lastname", "surname", "familyName")
	if first == "" || last == "" {
		full := valueAsString(values, "fullName", "full_name", "name")
		if full == "" {
			for key, raw := range values {
				lowerKey := strings.ToLower(strings.TrimSpace(key))
				if !strings.Contains(lowerKey, "name") {
					continue
				}
				candidate := strings.TrimSpace(fmt.Sprint(raw))
				if candidate == "" || emailRe.MatchString(candidate) {
					continue
				}
				full = candidate
				break
			}
		}
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

	role := normalizeLeadershipRoleInput(valueAsString(
		values,
		"role",
		"leadershipRole",
		"leadership_role",
		"leadershipCategory",
		"leadership_category",
		"position",
		"title",
	))

	emailAddr := valueAsString(values, "email", "contactEmail", "emailAddress", "email_address")
	phone := valueAsString(values, "phone", "contactPhone", "contactNumber", "phoneNumber", "contact_number")
	bio := valueAsString(values, "bio", "biography", "notes", "note", "comment", "message", "about", "profile")
	imageURL := valueAsString(
		values,
		"imageUrl",
		"image_url",
		"image",
		"profileImage",
		"profile_image",
		"photoUrl",
		"photo_url",
		"avatar",
		"picture",
		"photo",
	)
	birthday := valueAsString(values, "birthday", "birthDate", "birth_date", "dob", "dateOfBirth", "date_of_birth")
	anniversary := valueAsString(values, "anniversary", "weddingAnniversary", "wedding_anniversary", "anniversaryDate", "anniversary_date")

	// Never save raw base64 data images as leadership image URLs.
	// Public form uploads should upload first, then submit the returned public URL.
	if isDataImageValue(imageURL) {
		imageURL = ""
	}

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
		if full == "" {
			for key, raw := range values {
				lowerKey := strings.ToLower(strings.TrimSpace(key))
				if !strings.Contains(lowerKey, "name") {
					continue
				}
				candidate := strings.TrimSpace(fmt.Sprint(raw))
				if candidate == "" || emailRe.MatchString(candidate) {
					continue
				}
				full = candidate
				break
			}
		}
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
	if strings.TrimSpace(first) != "" && strings.TrimSpace(last) == "" {
		last = "Member"
	}
	if strings.TrimSpace(first) == "" {
		emailValue := strings.TrimSpace(valueAsString(values, "email", "contactEmail", "emailAddress", "email_address"))
		if emailValue != "" {
			local := emailValue
			if at := strings.Index(local, "@"); at > 0 {
				local = local[:at]
			}
			local = strings.TrimSpace(strings.ReplaceAll(local, ".", " "))
			if local != "" {
				token := strings.Fields(local)[0]
				runes := []rune(strings.ToLower(token))
				if len(runes) > 0 {
					runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
					first = string(runes)
				}
			}
		}
	}
	if strings.TrimSpace(first) == "" {
		first = "Church"
	}
	if strings.TrimSpace(last) == "" {
		last = "Member"
	}

	testimony := strings.TrimSpace(valueAsString(
		values,
		"testimony",
		"testimonyText",
		"testimony_text",
		"yourTestimony",
		"your_testimony",
		"message",
		"content",
		"description",
		"story",
		"note",
		"notes",
	))
	if testimony == "" {
		for key, raw := range values {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if strings.Contains(lowerKey, "testimony") || strings.Contains(lowerKey, "testimony_text") || strings.Contains(lowerKey, "story") {
				candidate := strings.TrimSpace(fmt.Sprint(raw))
				if candidate != "" {
					testimony = candidate
					break
				}
			}
		}
	}
	if testimony == "" {
		longest := ""
		for key, raw := range values {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if strings.Contains(lowerKey, "email") ||
				strings.Contains(lowerKey, "phone") ||
				strings.Contains(lowerKey, "name") ||
				strings.Contains(lowerKey, "consent") ||
				strings.Contains(lowerKey, "anonymous") ||
				strings.Contains(lowerKey, "role") {
				continue
			}
			candidate := strings.TrimSpace(fmt.Sprint(raw))
			if candidate == "" || emailRe.MatchString(candidate) {
				continue
			}
			if len(candidate) > len(longest) {
				longest = candidate
			}
		}
		if longest != "" {
			testimony = longest
		}
	}
	if testimony == "" {
		return nil, errors.New("missing required testimonial fields")
	}
	if countWords(testimony) > 400 {
		testimony = truncateWords(testimony, 400)
	}

	imageURL := strings.TrimSpace(valueAsString(values, "imageUrl", "image", "profileImage", "profile_image", "photo"))
	imageAssetID := strings.TrimSpace(valueAsString(values, "imageAssetId", "image_asset_id", "assetId", "asset_id"))
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
	if imageAssetID != "" {
		req.ImageAssetID = &imageAssetID
	}

	return req, nil
}

func buildLenientTestimonialRequest(values map[string]any) *models.CreateTestimonialRequest {
	first := strings.TrimSpace(valueAsString(values, "firstName", "first_name", "fullName", "full_name", "name"))
	last := strings.TrimSpace(valueAsString(values, "lastName", "last_name"))

	if first == "" {
		first = "Church"
	}
	if last == "" {
		last = "Member"
	}

	testimony := strings.TrimSpace(valueAsString(
		values,
		"testimony",
		"testimonyText",
		"testimony_text",
		"message",
		"content",
		"description",
		"story",
		"note",
		"notes",
	))

	if testimony == "" {
		for key, raw := range values {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if strings.Contains(lowerKey, "email") ||
				strings.Contains(lowerKey, "phone") ||
				strings.Contains(lowerKey, "name") ||
				strings.Contains(lowerKey, "consent") ||
				strings.Contains(lowerKey, "anonymous") ||
				strings.Contains(lowerKey, "role") {
				continue
			}
			candidate := strings.TrimSpace(fmt.Sprint(raw))
			if candidate != "" {
				testimony = candidate
				break
			}
		}
	}

	if testimony == "" {
		return nil
	}

	if countWords(testimony) > 400 {
		testimony = truncateWords(testimony, 400)
	}

	isAnonymousRaw := strings.ToLower(strings.TrimSpace(valueAsString(values, "isAnonymous", "anonymous")))
	isAnonymous := isAnonymousRaw == "true" || isAnonymousRaw == "1" || isAnonymousRaw == "yes"

	req := &models.CreateTestimonialRequest{
		FirstName:   first,
		LastName:    last,
		Testimony:   testimony,
		IsAnonymous: isAnonymous,
	}

	imageURL := strings.TrimSpace(valueAsString(values, "imageUrl", "image", "profileImage", "profile_image", "photo"))
	if imageURL != "" {
		req.ImageURL = &imageURL
	}
	imageAssetID := strings.TrimSpace(valueAsString(values, "imageAssetId", "image_asset_id", "assetId", "asset_id"))
	if imageAssetID != "" {
		req.ImageAssetID = &imageAssetID
	}

	return req
}

func normalizeLeadershipRoleInput(value string) models.LeadershipRole {
	clean := strings.ToLower(strings.TrimSpace(value))
	clean = strings.NewReplacer("-", "_", " ", "_").Replace(clean)
	clean = regexp.MustCompile(`_+`).ReplaceAllString(clean, "_")
	clean = strings.Trim(clean, "_")

	switch clean {
	case string(models.LeadershipRoleSeniorPastor), "pastor", "senior", "lead_pastor", "head_pastor":
		return models.LeadershipRoleSeniorPastor
	case string(models.LeadershipRoleAssociatePastor), "associate", "assistant_pastor", "assistant":
		return models.LeadershipRoleAssociatePastor
	case string(models.LeadershipRoleDeacon):
		return models.LeadershipRoleDeacon
	case string(models.LeadershipRoleDeaconess):
		return models.LeadershipRoleDeaconess
	case string(models.LeadershipRoleReverend), "rev":
		return models.LeadershipRoleReverend
	default:
		return models.LeadershipRoleAssociatePastor
	}
}

func truncateWords(value string, maxWords int) string {
	if maxWords <= 0 {
		return strings.TrimSpace(value)
	}
	words := strings.Fields(value)
	if len(words) <= maxWords {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(strings.Join(words[:maxWords], " "))
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
	wanted := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if normalized := normalizeFormValueKey(k); normalized != "" {
			wanted[normalized] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return ""
	}
	for k, v := range values {
		if _, ok := wanted[normalizeFormValueKey(k)]; !ok {
			continue
		}
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
	return ""
}

func normalizeFormValueKey(value string) string {
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
