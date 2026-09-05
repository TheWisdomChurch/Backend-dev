package email

import (
	"html"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
)

// RenderFormEmailContent is the single place that turns a form's structured
// response-email content into HTML/text. It is built entirely from this
// package's shared shell and component helpers (renderEmailShell,
// renderHeading, renderButton, renderInfoGrid, ...) — the same ones every
// other outbound email uses — so a redesign here (dark mode, spacing,
// corners, whatever) reaches every form's response email the next time its
// template is saved, with nothing to hand-port anywhere else. The admin
// portal sends FormEmailContent and nothing else; it must never go back to
// building its own copy of this HTML.
//
// The literal Go template actions below ({{.RecipientName}},
// {{if .RegistrationCode}}...{{end}}, etc.) are intentional: the returned
// HTML is stored as-is and executed against per-recipient data at send time
// by internal/service.renderDBTemplate, exactly like every other DB-stored
// template.
func RenderFormEmailContent(b Branding, content models.FormEmailContent) (htmlBody string, textBody string) {
	b = normalizeBranding(b)

	heading := strings.TrimSpace(content.Heading)
	if heading == "" {
		heading = "Registration Confirmed"
	}

	// Mirrors RenderFormResponseEmail's topLinks block.
	topLinks := "<tr><td style=\"padding:14px 40px 0;font-family:" + fontStack + ";\">" +
		"{{if .SubscribeURL}}<a href=\"{{.SubscribeURL}}\" style=\"color:" + colorInk + ";font-weight:700;text-decoration:underline;\">subscribe</a>{{end}}" +
		"{{if .SubscribeURL}}{{if .UnsubscribeURL}}&nbsp;|&nbsp;{{end}}{{end}}" +
		"{{if .UnsubscribeURL}}<a href=\"{{.UnsubscribeURL}}\" style=\"color:" + colorInk + ";font-weight:700;text-decoration:underline;\">unsubscribe</a>{{end}}" +
		"</td></tr>"

	// Section order mirrors the "Add New Member" reference template: greeting
	// -> message -> scripture/spotlight callout -> a distinct "what happens
	// next" section -> resources/registration/calendar -> a signed closing.
	var body strings.Builder
	body.WriteString(topLinks)
	body.WriteString(renderHeroImageBlock(strings.TrimSpace(content.ImageURL), heading))
	body.WriteString(renderBodyOpen())
	body.WriteString(renderEyebrow(content.Eyebrow, ""))
	body.WriteString(renderHeading(heading))
	body.WriteString(renderParagraph("Hello {{.RecipientName}},"))
	body.WriteString(renderBodyClose())

	body.WriteString(renderBodyOpen())
	body.WriteString(renderMessageBlock(content))
	body.WriteString(renderBodyClose())

	if spotlight := strings.TrimSpace(content.SpotlightText); spotlight != "" {
		body.WriteString(renderBodyOpen())
		body.WriteString(renderQuoteBlock(nl2br(html.EscapeString(spotlight)), content.SpotlightLabel))
		body.WriteString(renderBodyClose())
	}

	if rows := calendarSummaryItems(content.CalendarEvent); len(rows) > 0 {
		body.WriteString(renderInfoGrid(rows))
	}

	if nextSteps := renderNextStepsBlock(content); nextSteps != "" {
		body.WriteString(nextSteps)
	}

	if resources := renderResourceLinksBlock(content.ResourceLinks); resources != "" {
		body.WriteString(resources)
	}

	if content.IncludeRegistrationCode {
		body.WriteString("<tr><td style=\"padding:0 40px 30px;\">{{if .RegistrationCode}}")
		body.WriteString(renderCodeBlock("Registration number", "{{.RegistrationCode}}"))
		body.WriteString("{{end}}</td></tr>")
	}

	if content.IncludeCalendarOptIn {
		body.WriteString(renderCalendarOptInBlock(content))
	}

	if closing := renderClosingBlock(b, content); closing != "" {
		body.WriteString(closing)
	}

	return renderEmailShellWithPreheader(b, "", strings.TrimSpace(content.Preheader), body.String()),
		buildFormEmailText(content, heading)
}

// renderNextStepsBlock renders the "What happens next?" section: a
// subheading, a paragraph, and — if a CTA is configured — the action button,
// all folded into one section rather than a separate floating button. Any
// piece may be present alone (e.g. just a CTA with no heading/text).
func renderNextStepsBlock(content models.FormEmailContent) string {
	heading := strings.TrimSpace(content.NextStepsHeading)
	text := strings.TrimSpace(content.NextStepsText)
	cta := renderCTAButtons(content)
	if heading == "" && text == "" && cta == "" {
		return ""
	}

	var out strings.Builder
	out.WriteString(renderBodyOpen())
	if heading != "" {
		out.WriteString("<h3 style=\"margin:0 0 10px;font-size:16px;font-weight:700;color:" + colorInk + ";\">" + html.EscapeString(heading) + "</h3>")
	}
	if text != "" {
		out.WriteString(renderParagraph(nl2br(html.EscapeString(text))))
	}
	if cta != "" {
		out.WriteString("<div style=\"margin-top:6px;\">" + cta + "</div>")
	}
	out.WriteString(renderBodyClose())
	return out.String()
}

// renderClosingBlock renders the final "Once again, welcome... God bless
// you richly." style paragraph plus a signed valediction, just before the
// standard footer.
func renderClosingBlock(b Branding, content models.FormEmailContent) string {
	message := strings.TrimSpace(content.ClosingMessage)
	if message == "" {
		return ""
	}

	signOff := strings.TrimSpace(content.SignOff)
	if signOff == "" {
		signOff = "With love,"
	}
	appName := strings.TrimSpace(b.AppName)
	if appName == "" {
		appName = "The Wisdom Church"
	}

	return renderBodyOpen() +
		renderParagraph(nl2br(html.EscapeString(message))) +
		"<p style=\"margin:14px 0 0;font-size:13px;color:" + colorMuted + ";\">" + html.EscapeString(signOff) + "<br>" + html.EscapeString(appName) + "</p>" +
		renderBodyClose()
}

func renderMessageBlock(content models.FormEmailContent) string {
	if messageHTML := strings.TrimSpace(content.MessageHTML); messageHTML != "" {
		// MessageHTML is admin-authored rich text (from the response email
		// editor's WYSIWYG field), not user input at send time — trusted the
		// same way every other admin-composed HTML block in this system is.
		return messageHTML
	}

	message := strings.TrimSpace(content.Message)
	if message == "" {
		message = "Thank you for registering."
	}
	return renderParagraph(nl2br(html.EscapeString(message)))
}

func nl2br(escaped string) string {
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

func calendarSummaryItems(event *models.FormEmailCalendarEvent) []infoItem {
	if event == nil {
		return nil
	}

	var loc *time.Location
	if tz := strings.TrimSpace(event.TimeZone); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	start, startOK := parseCalendarTime(event.StartAt, loc)
	end, endOK := parseCalendarTime(event.EndAt, loc)

	dateValue := ""
	timeValue := ""
	if startOK {
		dateValue = start.Format("Monday, January 2, 2006")
		if endOK && !end.Equal(start) && !sameDay(start, end) {
			dateValue += " - " + end.Format("Monday, January 2, 2006")
		}
		timeValue = start.Format("3:04 PM")
		if endOK {
			timeValue += " - " + end.Format("3:04 PM")
		}
	}

	return []infoItem{
		{Label: "Event", Value: strings.TrimSpace(event.Title)},
		{Label: "Date", Value: dateValue},
		{Label: "Time", Value: timeValue},
		{Label: "Venue", Value: strings.TrimSpace(event.Location)},
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func parseCalendarTime(value string, loc *time.Location) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	if loc != nil {
		t = t.In(loc)
	}
	return t, true
}

// renderResourceLinksBlock renders a bordered box listing each configured
// resource (flyer, document, guide, schedule) with a plain action link —
// the same bordered-box/hairline language as the rest of this package's
// components, kept local here since no other template needs a link list.
func renderResourceLinksBlock(links []models.FormEmailResourceLink) string {
	var items strings.Builder
	for _, resource := range links {
		label := strings.TrimSpace(resource.Label)
		url := strings.TrimSpace(resource.URL)
		if label == "" || url == "" {
			continue
		}
		description := strings.TrimSpace(resource.Description)
		descHTML := ""
		if description != "" {
			descHTML = "<div style=\"margin:4px 0 8px;font-size:13px;color:" + colorMuted + ";\">" + html.EscapeString(description) + "</div>"
		}
		items.WriteString("<div style=\"margin:0 0 14px;\">" +
			"<div style=\"font-size:15px;font-weight:700;color:" + colorInk + ";\">" + html.EscapeString(label) + "</div>" +
			descHTML +
			renderTextLink(resourceActionLabel(resource.Kind), url) +
			"</div>")
	}
	if items.Len() == 0 {
		return ""
	}
	return "<tr><td style=\"padding:0 40px 6px;\">" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\"><tr><td class=\"wc-callout\" style=\"background:" + colorAccentSurface + ";border:1px solid " + colorAccentBorder + ";border-radius:12px;padding:18px 20px;font-family:" + fontStack + ";\">" +
		"<div style=\"font-size:11px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:" + colorAccent + ";margin-bottom:12px;\">Resources</div>" +
		items.String() +
		"</td></tr></table></td></tr>"
}

func resourceActionLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "flyer":
		return "Download flyer"
	case "document":
		return "Download document"
	case "guide":
		return "Open guide"
	case "schedule":
		return "Open schedule"
	default:
		return "Open resource"
	}
}

func renderCTAButtons(content models.FormEmailContent) string {
	label := strings.TrimSpace(content.CTALabel)
	url := strings.TrimSpace(content.CTAURL)
	if label == "" || url == "" {
		return ""
	}
	return renderButton(label, url, "", "")
}

// renderCalendarOptInBlock mirrors RenderFormResponseEmail's calendar block:
// an explicit CalendarURL (set by the admin) takes precedence, otherwise the
// per-recipient {{.CalendarOptInURL}} generated at send time is used.
func renderCalendarOptInBlock(content models.FormEmailContent) string {
	label := strings.TrimSpace(content.CalendarLabel)
	if label == "" {
		label = "Add event to calendar"
	}

	explicitURL := strings.TrimSpace(content.CalendarURL)
	linkHTML := renderTextLink(label, "{{.CalendarOptInURL}}")
	guardOpen, guardClose := "{{if .CalendarOptInURL}}", "{{end}}"
	if explicitURL != "" {
		linkHTML = renderTextLink(label, explicitURL)
		guardOpen, guardClose = "", ""
	}

	return "<tr><td style=\"padding:0 40px 30px;\">" + guardOpen +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\"><tr><td class=\"wc-callout\" style=\"background:" + colorAccentSurface + ";border:1px solid " + colorAccentBorder + ";border-radius:12px;padding:18px 20px;font-family:" + fontStack + ";\">" +
		"<div style=\"font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + colorAccent + ";margin-bottom:10px;\">Save the date</div>" +
		linkHTML +
		"</td></tr></table>" + guardClose + "</td></tr>"
}

func buildFormEmailText(content models.FormEmailContent, heading string) string {
	var lines []string
	lines = append(lines, heading)

	if eyebrow := strings.TrimSpace(content.Eyebrow); eyebrow != "" {
		lines = append(lines, "", eyebrow)
	}

	lines = append(lines, "", "Hello {{.RecipientName}},")

	if messageHTML := strings.TrimSpace(content.MessageHTML); messageHTML != "" {
		lines = append(lines, "", htmlToText(messageHTML))
	} else if message := strings.TrimSpace(content.Message); message != "" {
		lines = append(lines, "", message)
	} else {
		lines = append(lines, "", "Thank you for registering.")
	}

	if content.CalendarEvent != nil {
		lines = append(lines, "", "Event details:")
		for _, item := range calendarSummaryItems(content.CalendarEvent) {
			if strings.TrimSpace(item.Value) != "" {
				lines = append(lines, item.Label+": "+item.Value)
			}
		}
	}

	if heading := strings.TrimSpace(content.NextStepsHeading); heading != "" {
		lines = append(lines, "", heading)
	}
	if text := strings.TrimSpace(content.NextStepsText); text != "" {
		lines = append(lines, "", text)
	}

	if strings.TrimSpace(content.CTALabel) != "" && strings.TrimSpace(content.CTAURL) != "" {
		lines = append(lines, "", content.CTALabel+": "+content.CTAURL)
	}

	for _, resource := range content.ResourceLinks {
		if strings.TrimSpace(resource.Label) == "" || strings.TrimSpace(resource.URL) == "" {
			continue
		}
		lines = append(lines, "", resource.Label+": "+resource.URL)
	}

	if content.IncludeRegistrationCode {
		lines = append(lines, "", "{{if .RegistrationCode}}Registration Number: {{.RegistrationCode}}{{end}}")
	}

	if content.IncludeCalendarOptIn {
		calendarURL := strings.TrimSpace(content.CalendarURL)
		if calendarURL == "" {
			calendarURL = "{{.CalendarOptInURL}}"
		}
		label := strings.TrimSpace(content.CalendarLabel)
		if label == "" {
			label = "Add event to calendar"
		}
		lines = append(lines, "", label+": "+calendarURL)
	}

	if message := strings.TrimSpace(content.ClosingMessage); message != "" {
		signOff := strings.TrimSpace(content.SignOff)
		if signOff == "" {
			signOff = "With love,"
		}
		lines = append(lines, "", message, "", signOff)
	}

	if footer := strings.TrimSpace(content.FooterNote); footer != "" {
		lines = append(lines, "", footer)
	}

	return strings.Join(lines, "\n")
}

func htmlToText(value string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"</p>", "\n\n", "</div>", "\n\n", "</li>", "\n",
	)
	value = replacer.Replace(value)
	var out strings.Builder
	inTag := false
	for _, r := range value {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	return html.UnescapeString(strings.TrimSpace(out.String()))
}
