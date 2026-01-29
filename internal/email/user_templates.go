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
