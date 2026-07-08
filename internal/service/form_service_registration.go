// internal/service/form_service_registration.go
package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"wisdomHouse-backend/internal/email"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
)

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
		applog.L().Warn("failed to send registration code email", "to", emailAddr, "error", err)
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

	isTestimonial := false
	if settings != nil && settings.SubmissionTarget != nil {
		isTestimonial = strings.EqualFold(strings.TrimSpace(*settings.SubmissionTarget), "testimonial")
	}
	if !isTestimonial && settings != nil && settings.FormType != nil {
		isTestimonial = strings.EqualFold(strings.TrimSpace(*settings.FormType), "testimonial")
	}

	subject := "Registration received"
	if formTitle != "" {
		subject = fmt.Sprintf("Registration received: %s", formTitle)
	}
	if isTestimonial {
		subject = "Testimony received"
		if formTitle != "" {
			subject = fmt.Sprintf("Testimony received: %s", formTitle)
		}
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
	if isTestimonial {
		calendarOptInURL = ""
		googleCalendarURL = ""
		calendarICSURL = ""
		code = ""
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
				applog.L().Warn("response email DB template render failed", "template_id", tpl.ID, "form_id", formID, "error", err)
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
			applog.L().Warn("response email remote template render failed", "template_key", templateKey, "form_id", formID, "error", err)
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
		applog.L().Warn("failed to send form response email", "to", addr, "template_key", templateKey, "form_id", formID, "error", err)
	}
}
