package email

import (
	"html"
	"strings"
)

type PasswordChangedTemplateData struct {
	Branding  Branding
	Email     string
	Timestamp string
	LoginURL  string
}

func RenderPasswordChangedEmail(data PasswordChangedTemplateData) string {
	b := normalizeBranding(data.Branding)
	loginURL := strings.TrimSpace(data.LoginURL)
	if loginURL == "" && data.Branding.FrontendURL != "" {
		loginURL = strings.TrimSpace(data.Branding.FrontendURL)
	}

	actionBlock := ""
	if loginURL != "" {
		actionBlock = "<a href=\"" + html.EscapeString(loginURL) + "\" style=\"display:inline-block;margin-top:10px;padding:12px 18px;background:#111827;color:#ffffff;text-decoration:none;border-radius:10px;font-size:14px;font-weight:700;\">Return to login</a>"
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#111827;background:#f8f9fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border:1px solid #e5e7eb;border-radius:18px;padding:32px;\">" +
		renderLogoBlock(b) +
		"<h2 style=\"margin:0 0 12px;font-size:22px;color:#0f172a;\">Your " + html.EscapeString(b.AppName) + " password was changed</h2>" +
		"<p style=\"margin:0 0 14px;font-size:15px;color:#334155;\">This is a confirmation that the password for your account was changed.</p>" +
		"<div style=\"margin:16px 0;padding:16px;background:#f8fafc;border:1px solid #e2e8f0;border-radius:12px;font-size:14px;color:#334155;\">" +
		"<strong>Email:</strong> " + html.EscapeString(strings.TrimSpace(data.Email)) + "<br/>" +
		"<strong>Time (UTC):</strong> " + html.EscapeString(strings.TrimSpace(data.Timestamp)) +
		"</div>" +
		"<p style=\"margin:0 0 10px;font-size:13px;color:#6b7280;\">If you did not make this change, please reset your password immediately.</p>" +
		actionBlock +
		footerBlock(b) +
		"</div></body></html>"
}
