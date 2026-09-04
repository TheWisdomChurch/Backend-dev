package email

import (
	"html"
	"strings"
)

type RegistrationTemplateData struct {
	Branding      Branding
	RecipientName string
	ActionURL     string
	Message       string
	HeroImageURL  string
}

func RenderRegistrationEmail(data RegistrationTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}
	customMessage := strings.TrimSpace(data.Message)
	messageBlock := ""
	if customMessage != "" {
		messageBlock = renderParagraph(html.EscapeString(customMessage))
	}

	body := renderHeroImageBlock(data.HeroImageURL, b.AppName+" welcome") +
		renderBodyOpen() +
		renderHeading("Welcome to "+b.AppName) +
		renderParagraph("Hi "+name+", thanks for registering with "+html.EscapeString(b.AppName)+".") +
		messageBlock +
		renderBodyClose() +
		renderActionRow(renderButton("Complete your profile", data.ActionURL, "", ""))

	return renderEmailShell(b, "", body)
}

type BirthdayTemplateData struct {
	Branding      Branding
	RecipientName string
	BirthdayDate  string
	Message       string
	HeroImageURL  string
}

func RenderBirthdayEmail(data BirthdayTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "friend"
	} else {
		name = html.EscapeString(name)
	}
	dateLine := ""
	if strings.TrimSpace(data.BirthdayDate) != "" {
		dateLine = "<p style=\"margin:0 0 16px;font-size:14px;color:" + colorMuted + ";\">Celebrating on " + html.EscapeString(strings.TrimSpace(data.BirthdayDate)) + "</p>"
	}
	customMessage := strings.TrimSpace(data.Message)
	message := "Wishing you a joyful birthday filled with peace and blessings."
	if customMessage != "" {
		message = html.EscapeString(customMessage)
	} else {
		message = html.EscapeString(message)
	}

	body := renderHeroImageBlock(data.HeroImageURL, "Happy birthday") +
		renderBodyOpen() +
		renderHeading("Happy Birthday, "+name+"!") +
		renderParagraph(message) +
		dateLine +
		"<p style=\"margin:0;font-size:14px;color:" + colorMuted + ";\">With love,</p>" +
		"<p style=\"margin:4px 0 0;font-size:14px;font-weight:600;color:" + colorInk + ";\">" + html.EscapeString(b.AppName) + " Team</p>" +
		renderBodyClose()

	return renderEmailShell(b, "", body)
}

type RegistrationCodeTemplateData struct {
	Branding      Branding
	RecipientName string
	EventName     string
	Code          string
	Message       string
}

func RenderRegistrationCodeEmail(data RegistrationCodeTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}
	eventName := strings.TrimSpace(data.EventName)
	if eventName == "" {
		eventName = "your registration"
	} else {
		eventName = html.EscapeString(eventName)
	}
	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = "Your registration code is ready."
	} else {
		message = html.EscapeString(message)
	}

	body := renderBodyOpen() +
		renderHeading("Hello "+name+",") +
		renderParagraph(message) +
		"<p style=\"margin:0 0 8px;font-size:15px;color:" + colorBody + ";\">Event: <strong style=\"color:" + colorInk + ";\">" + eventName + "</strong></p>" +
		renderBodyClose() +
		"<tr><td style=\"padding:8px 40px 32px;\">" + renderCodeBlock("Registration code", data.Code) + "</td></tr>"

	return renderEmailShell(b, "", body)
}

type FormResponseTemplateData struct {
	Branding          Branding
	RecipientName     string
	FormTitle         string
	RegistrationCode  string
	Message           string
	HeroImageURL      string
	CalendarOptInURL  string
	GoogleCalendarURL string
	CalendarICSURL    string
	SubscribeURL      string
	UnsubscribeURL    string
}

type EventReminderTemplateData struct {
	Branding          Branding
	RecipientName     string
	EventTitle        string
	EventDate         string
	EventTime         string
	EventLocation     string
	RegistrationCode  string
	GoogleCalendarURL string
	CalendarICSURL    string
	UnsubscribeURL    string
}

type LeadershipStatusTemplateData struct {
	Branding      Branding
	RecipientName string
	Role          string
	Message       string
	HeroImageURL  string
}

type AnniversaryTemplateData struct {
	Branding        Branding
	RecipientName   string // may already be "David & Sarah" when a couple
	SpouseName      string // optional; only used to compose a greeting when RecipientName is a single name
	AnniversaryDate string
	Message         string
	ScriptureHTML   string // optional blessing verse, rendered as a quote block
	HeroImageURL    string
}

func RenderLeadershipApprovedEmail(data LeadershipStatusTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}
	role := strings.TrimSpace(data.Role)
	if role == "" {
		role = "leadership"
	}
	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = "Your leadership application has been approved."
	}

	body := renderHeroImageBlock(data.HeroImageURL, "Leadership approved") +
		renderBodyOpen() +
		renderHeading("Hello "+name+",") +
		renderParagraph(html.EscapeString(message)) +
		"<p style=\"margin:0 0 12px;font-size:15px;color:" + colorBody + ";\">Role: <strong style=\"color:" + colorInk + ";\">" + html.EscapeString(role) + "</strong></p>" +
		renderBodyClose()

	return renderEmailShell(b, "", body)
}

func RenderLeadershipDeclinedEmail(data LeadershipStatusTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}
	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = "Your leadership application has been declined for now."
	}

	body := renderHeroImageBlock(data.HeroImageURL, "Leadership update") +
		renderBodyOpen() +
		renderHeading("Hello "+name+",") +
		renderParagraph(html.EscapeString(message)) +
		renderBodyClose()

	return renderEmailShell(b, "", body)
}

func RenderAnniversaryEmail(data AnniversaryTemplateData) string {
	b := normalizeBranding(data.Branding)

	// RecipientName may already be a couple ("David & Sarah"). If it's a single
	// name and a SpouseName is supplied, compose the couple greeting here.
	name := strings.TrimSpace(data.RecipientName)
	spouse := strings.TrimSpace(data.SpouseName)
	if name != "" && spouse != "" && !strings.Contains(name, "&") {
		name = name + " & " + spouse
	}
	if name == "" {
		name = "friend"
	}
	// name is passed into renderHeading below, which escapes the whole
	// heading string itself — escaping here too would double-escape "&".

	date := strings.TrimSpace(data.AnniversaryDate)
	dateLine := ""
	if date != "" {
		dateLine = "<p style=\"margin:0 0 16px;font-size:14px;color:" + colorMuted + ";\">Celebrating on " + html.EscapeString(date) + "</p>"
	}

	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = "On behalf of the whole church family, we celebrate the covenant you have kept and the home you have built together. May this new year of marriage bring you deeper joy, renewed strength, and abundant grace."
	}

	scripture := ""
	if s := strings.TrimSpace(data.ScriptureHTML); s != "" {
		scripture = renderQuoteBlock(s, "")
	}

	body := renderHeroImageBlock(data.HeroImageURL, "Wedding anniversary") +
		renderBodyOpen() +
		renderEyebrow("A moment worth celebrating", "") +
		renderHeading("Happy Wedding Anniversary, "+name+"!") +
		renderParagraph(html.EscapeString(message)) +
		dateLine +
		scripture +
		"<p style=\"margin:18px 0 0;font-size:14px;color:" + colorMuted + ";\">With love and prayers,</p>" +
		"<p style=\"margin:4px 0 0;font-size:14px;font-weight:600;color:" + colorInk + ";\">" + html.EscapeString(b.AppName) + "</p>" +
		renderBodyClose()

	return renderEmailShellWithPreheader(b, "", "Celebrating your wedding anniversary — with love from "+b.AppName+".", body)
}

// RenderFormResponseEmail renders a registration confirmation for form submissions.
func RenderFormResponseEmail(data FormResponseTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}

	formTitle := strings.TrimSpace(data.FormTitle)
	if formTitle == "" {
		formTitle = "your registration"
	} else {
		formTitle = html.EscapeString(formTitle)
	}

	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = "Thank you for registering for " + formTitle + "."
	} else {
		message = html.EscapeString(message)
	}

	topLinks := ""
	if strings.TrimSpace(data.SubscribeURL) != "" || strings.TrimSpace(data.UnsubscribeURL) != "" {
		var links []string
		if strings.TrimSpace(data.SubscribeURL) != "" {
			links = append(links, "<a href=\""+html.EscapeString(strings.TrimSpace(data.SubscribeURL))+"\" style=\"color:"+colorInk+";font-weight:700;text-decoration:underline;\">subscribe</a>")
		}
		if strings.TrimSpace(data.UnsubscribeURL) != "" {
			links = append(links, "<a href=\""+html.EscapeString(strings.TrimSpace(data.UnsubscribeURL))+"\" style=\"color:"+colorInk+";font-weight:700;text-decoration:underline;\">unsubscribe</a>")
		}
		topLinks = "<tr><td style=\"padding:14px 40px 0;font-family:" + fontStack + ";\"><p style=\"margin:0;font-size:13px;color:" + colorInk + ";\">" + strings.Join(links, " &nbsp;|&nbsp; ") + "</p></td></tr>"
	}

	codeBlock := ""
	if code := strings.TrimSpace(data.RegistrationCode); code != "" {
		codeBlock = "<tr><td style=\"padding:16px 40px 0;\">" + renderCodeBlock("Registration number", code) + "</td></tr>"
	}

	calendarBlock := ""
	if strings.TrimSpace(data.CalendarOptInURL) != "" || strings.TrimSpace(data.GoogleCalendarURL) != "" || strings.TrimSpace(data.CalendarICSURL) != "" {
		confirmLink := strings.TrimSpace(data.CalendarOptInURL)
		if confirmLink == "" {
			confirmLink = strings.TrimSpace(data.GoogleCalendarURL)
		}
		var inner strings.Builder
		inner.WriteString("<div style=\"font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + colorMuted + ";margin-bottom:10px;\">Calendar reminders</div>")
		inner.WriteString("<p style=\"margin:0 0 12px;font-size:14px;color:" + colorBody + ";\">Would you like calendar reminders before the event?</p>")
		if confirmLink != "" {
			inner.WriteString(renderTextLink("Add event to calendar", confirmLink))
		}
		if strings.TrimSpace(data.CalendarICSURL) != "" {
			inner.WriteString("<p style=\"margin:10px 0 0;font-size:12px;color:" + colorFaint + ";\">Apple/Outlook: " + renderTextLink("Download .ics", strings.TrimSpace(data.CalendarICSURL)) + "</p>")
		}
		calendarBlock = "<tr><td style=\"padding:20px 40px 0;\"><table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"border:1px solid " + colorLine + ";\"><tr><td style=\"padding:18px 20px;font-family:" + fontStack + ";\">" + inner.String() + "</td></tr></table></td></tr>"
	}

	body := topLinks +
		renderHeroImageBlock(data.HeroImageURL, b.AppName+" registration") +
		renderBodyOpen() +
		renderHeading("Hi "+name+",") +
		renderParagraph(message) +
		renderBodyClose() +
		codeBlock +
		calendarBlock

	return renderEmailShell(b, "", body)
}

func RenderEventReminderEmail(data EventReminderTemplateData) string {
	b := normalizeBranding(data.Branding)

	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}

	title := strings.TrimSpace(data.EventTitle)
	if title == "" {
		title = "your event"
	} else {
		title = html.EscapeString(title)
	}

	codeBlock := ""
	if code := strings.TrimSpace(data.RegistrationCode); code != "" {
		codeBlock = "<tr><td style=\"padding:16px 40px 0;\">" + renderCodeBlock("Registration number", code) + "</td></tr>"
	}

	var calendarButtons strings.Builder
	if strings.TrimSpace(data.GoogleCalendarURL) != "" {
		calendarButtons.WriteString(renderButton("Open Google Calendar", strings.TrimSpace(data.GoogleCalendarURL), "", ""))
	}
	if strings.TrimSpace(data.CalendarICSURL) != "" {
		if calendarButtons.Len() > 0 {
			calendarButtons.WriteString("<div style=\"height:10px;line-height:10px;font-size:0;\">&nbsp;</div>")
		}
		calendarButtons.WriteString(renderOutlineButton("Download .ics", strings.TrimSpace(data.CalendarICSURL)))
	}
	calendarRow := ""
	if calendarButtons.Len() > 0 {
		calendarRow = "<tr><td style=\"padding:20px 40px 0;\">" + calendarButtons.String() + "</td></tr>"
	}

	unsubscribeRow := ""
	if strings.TrimSpace(data.UnsubscribeURL) != "" {
		unsubscribeRow = "<tr><td style=\"padding:16px 40px 0;font-family:" + fontStack + ";\"><p style=\"margin:0;font-size:12px;color:" + colorFaint + ";\">If you prefer not to receive reminders, <a href=\"" + html.EscapeString(strings.TrimSpace(data.UnsubscribeURL)) + "\" style=\"color:" + colorMuted + ";\">unsubscribe here</a>.</p></td></tr>"
	}

	body := renderBodyOpen() +
		renderEyebrow("Gentle reminder", "") +
		renderHeading("Reminder for tomorrow") +
		renderParagraph("Hello "+name+", this is a quick reminder for <strong style=\"color:"+colorInk+";\">"+title+"</strong>.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "Date", Value: strings.TrimSpace(data.EventDate)},
			{Label: "Time", Value: strings.TrimSpace(data.EventTime)},
			{Label: "Location", Value: strings.TrimSpace(data.EventLocation)},
		}) +
		codeBlock +
		calendarRow +
		unsubscribeRow

	return renderEmailShell(b, "", body)
}
