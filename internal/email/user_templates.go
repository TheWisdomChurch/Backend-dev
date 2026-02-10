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
	Branding         Branding
	RecipientName    string
	FormTitle        string
	EventTitle       string
	EventDate        string
	EventTime        string
	EventLocation    string
	RegistrationCode string
	Message          string
	FormURL          string
	HeroImageURL     string
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
		message = "Thanks for registering for " + formTitle + "."
	} else {
		message = html.EscapeString(message)
	}

	eventTitle := strings.TrimSpace(data.EventTitle)
	eventDate := strings.TrimSpace(data.EventDate)
	eventTime := strings.TrimSpace(data.EventTime)
	eventLocation := strings.TrimSpace(data.EventLocation)

	eventBlock := ""
	if eventTitle != "" || eventDate != "" || eventTime != "" || eventLocation != "" {
		if eventTitle != "" {
			eventBlock += "<p style=\"margin:0 0 6px;font-size:15px;color:#334155;\">Event: <strong>" + html.EscapeString(eventTitle) + "</strong></p>"
		}
		if eventDate != "" {
			eventBlock += "<p style=\"margin:0 0 6px;font-size:14px;color:#475569;\">Date: " + html.EscapeString(eventDate) + "</p>"
		}
		if eventTime != "" {
			eventBlock += "<p style=\"margin:0 0 6px;font-size:14px;color:#475569;\">Time: " + html.EscapeString(eventTime) + "</p>"
		}
		if eventLocation != "" {
			eventBlock += "<p style=\"margin:0 0 6px;font-size:14px;color:#475569;\">Location: " + html.EscapeString(eventLocation) + "</p>"
		}
	}

	codeBlock := ""
	if code := strings.TrimSpace(data.RegistrationCode); code != "" {
		codeBlock = "<div style=\"margin:14px 0;padding:12px 16px;background:#f1f5f9;border-radius:12px;display:inline-block;\">" +
			"<span style=\"font-size:18px;letter-spacing:1.5px;font-weight:700;\">" + html.EscapeString(code) + "</span>" +
			"</div>"
	}

	actionBlock := ""
	if formURL := strings.TrimSpace(data.FormURL); formURL != "" {
		actionBlock = "<a href=\"" + html.EscapeString(formURL) + "\" style=\"display:inline-block;margin-top:10px;padding:11px 18px;background:#1d4ed8;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:600;\">View registration</a>"
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
		eventBlock +
		codeBlock +
		actionBlock +
		footerBlock(b) +
		"</div></body></html>"
}
