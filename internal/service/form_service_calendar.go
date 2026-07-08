// internal/service/form_service_calendar.go
package service

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"

	"wisdomHouse-backend/internal/email"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
)

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
		applog.L().Warn("calendar token generation failed", "form_id", form.ID, "submission_id", submissionID, "error", err)
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
		applog.L().Warn("failed to persist calendar reminder", "form_id", form.ID, "submission_id", submissionID, "error", err)
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
		applog.L().Warn("failed to mark calendar opt-in", "id", row.ID, "error", err)
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
			applog.L().Warn("failed to send event reminder email", "to", addr, "error", err)
			continue
		}
		if err := s.reminderRepo.MarkReminderSent(item.ID, now.UTC()); err != nil {
			failed++
			applog.L().Warn("failed to mark event reminder sent", "id", item.ID, "error", err)
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
