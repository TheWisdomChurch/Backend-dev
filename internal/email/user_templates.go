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
	actionURL := html.EscapeString(strings.TrimSpace(data.ActionURL))
	customMessage := strings.TrimSpace(data.Message)
	messageBlock := ""
	if customMessage != "" {
		messageBlock = "<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">" + html.EscapeString(customMessage) + "</p>"
	}

	logoBlock := renderLogoBlock(b)
	heroBlock := renderHeroImageBlock(data.HeroImageURL, b.AppName+" welcome")

	actionBlock := ""
	if actionURL != "" {
		actionBlock = "<a href=\"" + actionURL + "\" style=\"display:inline-block;margin-top:8px;padding:12px 18px;background:#1d4ed8;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:600;\">Complete your profile</a>"
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		heroBlock +
		"<h2 style=\"margin:0 0 12px;font-size:22px;\">Welcome to " + html.EscapeString(b.AppName) + "</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">Hi " + name + ", thanks for registering with " + html.EscapeString(b.AppName) + ".</p>" +
		messageBlock +
		actionBlock +
		footerBlock(b) +
		"</div></body></html>"
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
		dateLine = "<p style=\"margin:0 0 16px;font-size:14px;color:#64748b;\">Celebrating on " + html.EscapeString(strings.TrimSpace(data.BirthdayDate)) + "</p>"
	}
	customMessage := strings.TrimSpace(data.Message)
	messageBlock := "<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">Wishing you a joyful birthday filled with peace and blessings.</p>"
	if customMessage != "" {
		messageBlock = "<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">" + html.EscapeString(customMessage) + "</p>"
	}

	logoBlock := renderLogoBlock(b)
	heroBlock := renderHeroImageBlock(data.HeroImageURL, "Happy birthday")

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f8fafc;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		heroBlock +
		"<h2 style=\"margin:0 0 12px;font-size:24px;color:#0b2447;\">Happy Birthday, " + name + "!</h2>" +
		messageBlock +
		dateLine +
		"<p style=\"margin:0;font-size:14px;color:#475569;\">With love,</p>" +
		"<p style=\"margin:4px 0 0;font-size:14px;font-weight:600;color:#1f2933;\">" + html.EscapeString(b.AppName) + " Team</p>" +
		footerBlock(b) +
		"</div></body></html>"
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
	code := html.EscapeString(strings.TrimSpace(data.Code))
	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = "Your registration code is ready."
	}

	logoBlock := renderLogoBlock(b)

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f8fafc;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		"<h2 style=\"margin:0 0 12px;font-size:22px;color:#0b2447;\">Hello " + name + ",</h2>" +
		"<p style=\"margin:0 0 12px;font-size:15px;color:#334155;\">" + html.EscapeString(message) + "</p>" +
		"<p style=\"margin:0 0 8px;font-size:15px;color:#334155;\">Event: <strong>" + eventName + "</strong></p>" +
		"<div style=\"margin:16px 0;padding:14px 18px;background:#f1f5f9;border-radius:12px;display:inline-block;\">" +
		"<span style=\"font-size:20px;letter-spacing:2px;font-weight:700;\">" + code + "</span>" +
		"</div>" +
		footerBlock(b) +
		"</div></body></html>"
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
	RecipientName   string
	AnniversaryDate string
	Message         string
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

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		renderLogoBlock(b) +
		renderHeroImageBlock(data.HeroImageURL, "Leadership approved") +
		"<h2 style=\"margin:0 0 12px;font-size:22px;\">Hello " + name + ",</h2>" +
		"<p style=\"margin:0 0 12px;font-size:15px;color:#334155;\">" + html.EscapeString(message) + "</p>" +
		"<p style=\"margin:0 0 12px;font-size:15px;color:#334155;\">Role: <strong>" + html.EscapeString(role) + "</strong></p>" +
		footerBlock(b) +
		"</div></body></html>"
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

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		renderLogoBlock(b) +
		renderHeroImageBlock(data.HeroImageURL, "Leadership update") +
		"<h2 style=\"margin:0 0 12px;font-size:22px;\">Hello " + name + ",</h2>" +
		"<p style=\"margin:0 0 12px;font-size:15px;color:#334155;\">" + html.EscapeString(message) + "</p>" +
		footerBlock(b) +
		"</div></body></html>"
}

func RenderAnniversaryEmail(data AnniversaryTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "friend"
	} else {
		name = html.EscapeString(name)
	}
	date := strings.TrimSpace(data.AnniversaryDate)
	dateLine := ""
	if date != "" {
		dateLine = "<p style=\"margin:0 0 16px;font-size:14px;color:#64748b;\">Celebrating on " + html.EscapeString(date) + "</p>"
	}
	message := strings.TrimSpace(data.Message)
	if message == "" {
		message = "Warm wishes on your wedding anniversary."
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f8fafc;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		renderLogoBlock(b) +
		renderHeroImageBlock(data.HeroImageURL, "Wedding anniversary") +
		"<h2 style=\"margin:0 0 12px;font-size:24px;color:#0b2447;\">Happy Wedding Anniversary, " + name + "!</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">" + html.EscapeString(message) + "</p>" +
		dateLine +
		"<p style=\"margin:0;font-size:14px;color:#475569;\">With love,</p>" +
		"<p style=\"margin:4px 0 0;font-size:14px;font-weight:600;color:#1f2933;\">" + html.EscapeString(b.AppName) + " Team</p>" +
		footerBlock(b) +
		"</div></body></html>"
}

// RenderFormResponseEmail renders a simple registration confirmation for form submissions.
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

	codeBlock := ""
	if code := strings.TrimSpace(data.RegistrationCode); code != "" {
		codeBlock = "<div style=\"margin:16px 0 20px;padding:14px 16px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:12px;display:inline-block;\">" +
			"<div style=\"font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:#1d4ed8;font-weight:700;\">Registration Number</div>" +
			"<div style=\"font-size:20px;letter-spacing:1.6px;font-weight:800;color:#0b2447;margin-top:4px;\">" + html.EscapeString(code) + "</div>" +
			"</div>"
	}

	calendarBlock := ""
	if strings.TrimSpace(data.CalendarOptInURL) != "" || strings.TrimSpace(data.GoogleCalendarURL) != "" || strings.TrimSpace(data.CalendarICSURL) != "" {
		confirmLink := strings.TrimSpace(data.CalendarOptInURL)
		if confirmLink == "" {
			confirmLink = strings.TrimSpace(data.GoogleCalendarURL)
		}
		calendarBlock += "<div style=\"margin:20px 0 10px;padding:16px;background:#f8fafc;border:1px solid #e2e8f0;border-radius:12px;\">"
		calendarBlock += "<p style=\"margin:0 0 10px;font-size:14px;color:#334155;\">Would you like calendar reminders before the event?</p>"
		if confirmLink != "" {
			calendarBlock += "<a href=\"" + html.EscapeString(confirmLink) + "\" style=\"display:inline-block;padding:11px 16px;background:#0f172a;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:700;font-size:14px;\">Add Event To Calendar</a>"
		}
		if strings.TrimSpace(data.CalendarICSURL) != "" {
			calendarBlock += "<p style=\"margin:10px 0 0;font-size:12px;color:#64748b;\">Apple/Outlook: <a href=\"" + html.EscapeString(strings.TrimSpace(data.CalendarICSURL)) + "\" style=\"color:#1d4ed8;\">Download .ics</a></p>"
		}
		calendarBlock += "</div>"
	}

	subscriptionBlock := ""
	if strings.TrimSpace(data.SubscribeURL) != "" || strings.TrimSpace(data.UnsubscribeURL) != "" {
		subscriptionBlock = "<div style=\"margin:16px 0 0;padding-top:14px;border-top:1px solid #e5e7eb;\">"
		if strings.TrimSpace(data.SubscribeURL) != "" {
			subscriptionBlock += "<a href=\"" + html.EscapeString(strings.TrimSpace(data.SubscribeURL)) + "\" style=\"display:inline-block;margin-right:10px;padding:8px 12px;background:#16a34a;color:#ffffff;text-decoration:none;border-radius:8px;font-size:13px;font-weight:700;\">Subscribe</a>"
		}
		if strings.TrimSpace(data.UnsubscribeURL) != "" {
			subscriptionBlock += "<a href=\"" + html.EscapeString(strings.TrimSpace(data.UnsubscribeURL)) + "\" style=\"display:inline-block;padding:8px 12px;background:#e2e8f0;color:#0f172a;text-decoration:none;border-radius:8px;font-size:13px;font-weight:700;\">Unsubscribe</a>"
		}
		subscriptionBlock += "</div>"
	}

	logoBlock := renderLogoBlock(b)
	heroBlock := renderHeroImageBlock(data.HeroImageURL, b.AppName+" registration")

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		heroBlock +
		"<h2 style=\"margin:0 0 12px;font-size:22px;\">Hi " + name + ",</h2>" +
		"<p style=\"margin:0 0 14px;font-size:15px;color:#334155;\">" + message + "</p>" +
		codeBlock +
		calendarBlock +
		subscriptionBlock +
		footerBlock(b) +
		"</div></body></html>"
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

	dateLine := html.EscapeString(strings.TrimSpace(data.EventDate))
	timeLine := html.EscapeString(strings.TrimSpace(data.EventTime))
	locationLine := html.EscapeString(strings.TrimSpace(data.EventLocation))

	codeBlock := ""
	if code := strings.TrimSpace(data.RegistrationCode); code != "" {
		codeBlock = "<div style=\"margin:14px 0;padding:12px 16px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:12px;display:inline-block;\">" +
			"<div style=\"font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:#1d4ed8;font-weight:700;\">Registration Number</div>" +
			"<div style=\"font-size:18px;letter-spacing:1.2px;font-weight:800;color:#0b2447;margin-top:4px;\">" + html.EscapeString(code) + "</div>" +
			"</div>"
	}

	calendarActions := ""
	if strings.TrimSpace(data.GoogleCalendarURL) != "" {
		calendarActions += "<a href=\"" + html.EscapeString(strings.TrimSpace(data.GoogleCalendarURL)) + "\" style=\"display:inline-block;margin-right:10px;padding:10px 14px;background:#0f172a;color:#ffffff;text-decoration:none;border-radius:9px;font-weight:700;font-size:13px;\">Open Google Calendar</a>"
	}
	if strings.TrimSpace(data.CalendarICSURL) != "" {
		calendarActions += "<a href=\"" + html.EscapeString(strings.TrimSpace(data.CalendarICSURL)) + "\" style=\"display:inline-block;padding:10px 14px;background:#e2e8f0;color:#0f172a;text-decoration:none;border-radius:9px;font-weight:700;font-size:13px;\">Download .ics</a>"
	}

	unsubscribeBlock := ""
	if strings.TrimSpace(data.UnsubscribeURL) != "" {
		unsubscribeBlock = "<p style=\"margin:18px 0 0;font-size:12px;color:#94a3b8;\">If you prefer not to receive reminders, <a href=\"" + html.EscapeString(strings.TrimSpace(data.UnsubscribeURL)) + "\" style=\"color:#64748b;\">unsubscribe here</a>.</p>"
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		renderLogoBlock(b) +
		"<h2 style=\"margin:0 0 10px;font-size:24px;color:#0b2447;\">Gentle reminder for tomorrow</h2>" +
		"<p style=\"margin:0 0 14px;font-size:15px;color:#334155;\">Hello " + name + ", this is a quick reminder for <strong>" + title + "</strong>.</p>" +
		"<div style=\"margin:0 0 8px;font-size:14px;color:#334155;\"><strong>Date:</strong> " + dateLine + "</div>" +
		"<div style=\"margin:0 0 8px;font-size:14px;color:#334155;\"><strong>Time:</strong> " + timeLine + "</div>" +
		"<div style=\"margin:0 0 8px;font-size:14px;color:#334155;\"><strong>Location:</strong> " + locationLine + "</div>" +
		codeBlock +
		calendarActions +
		unsubscribeBlock +
		footerBlock(b) +
		"</div></body></html>"
}
