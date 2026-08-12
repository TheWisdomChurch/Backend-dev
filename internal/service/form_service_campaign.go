// internal/service/form_service_campaign.go
package service

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"wisdomHouse-backend/internal/email"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
)

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
	if form == nil {
		return nil, errors.New("form not found")
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
			applog.L().Warn("failed to render form campaign email", "to", addr, "form_id", form.ID, "error", err)
			continue
		}

		if err := s.sendFormCampaignMessage(addr, subject, content); err != nil {
			resp.Failed++
			resp.FailedRecipients = appendFailedRecipient(resp.FailedRecipients, addr)
			applog.L().Warn("failed to send form campaign email", "to", addr, "form_id", form.ID, "error", err)
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
		applog.L().Warn("failed to persist form campaign delivery", "form_id", form.ID, "error", saveErr)
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
