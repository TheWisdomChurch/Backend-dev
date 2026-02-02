package email

import (
	"html"
	"strings"
)

type BirthdayTemplateData struct {
	Branding      Branding
	RecipientName string
	Department    string
}

// RenderBirthdayEmail generates a simple celebratory email.
func RenderBirthdayEmail(data BirthdayTemplateData) string {
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "Team member"
	}

	dept := strings.TrimSpace(data.Department)
	if dept != "" {
		dept = " (" + html.EscapeString(dept) + ")"
	}

	return "<!DOCTYPE html><html><body style='font-family:Arial,sans-serif;line-height:1.6;color:#111'>" +
		"<div style='max-width:640px;margin:0 auto;padding:24px;border:1px solid #eee;border-radius:12px;background:#fff'>" +
		"<p style='margin:0 0 12px;font-size:16px;'>Happy Birthday, <strong>" + html.EscapeString(name) + "</strong>" + dept + "!</p>" +
		"<p style='margin:0 0 12px;'>We celebrate you today and appreciate all you bring to the team.</p>" +
		"<p style='margin:0 0 12px;'>May your year ahead be filled with joy, growth, and impact.</p>" +
		"<p style='margin:18px 0 0;font-size:13px;color:#666;'>With love,<br/>" + html.EscapeString(data.Branding.AppName) + "</p>" +
		"</div></body></html>"
}
