package email

import (
	"html"
	"strings"
)

// SecurityAlertTemplateData is the data fed into the security alert email.
type SecurityAlertTemplateData struct {
	Branding  Branding
	Email     string
	Reason    string
	IP        string
	UserAgent string
	Timestamp string
	ManageURL string
}

// RenderSecurityAlertEmail renders a simple HTML email notifying of a security event.
func RenderSecurityAlertEmail(data SecurityAlertTemplateData) string {
	b := normalizeBranding(data.Branding)

	actionBlock := ""
	if manageURL := strings.TrimSpace(data.ManageURL); manageURL != "" {
		actionBlock = "<a href=\"" + html.EscapeString(manageURL) + "\" style=\"display:inline-block;margin-top:10px;padding:12px 18px;background:#111827;color:#ffffff;text-decoration:none;border-radius:10px;font-size:14px;font-weight:700;\">Review devices</a>"
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#111827;background:#f8f9fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border:1px solid #e5e7eb;border-radius:18px;padding:32px;\">" +
		renderLogoBlock(b) +
		"<h2 style=\"margin:0 0 12px;font-size:22px;color:#0f172a;\">" + html.EscapeString(b.AppName) + " account security</h2>" +
		"<p style=\"margin:0 0 14px;font-size:15px;color:#334155;\">We detected activity that needs your review.</p>" +
		"<div style=\"margin:16px 0;padding:16px;background:#f8fafc;border:1px solid #e2e8f0;border-radius:12px;font-size:14px;color:#334155;\">" +
		"<strong>Reason:</strong> " + html.EscapeString(strings.TrimSpace(data.Reason)) + "<br/>" +
		"<strong>Email:</strong> " + html.EscapeString(strings.TrimSpace(data.Email)) + "<br/>" +
		"<strong>IP:</strong> " + html.EscapeString(strings.TrimSpace(data.IP)) + "<br/>" +
		"<strong>Browser:</strong> " + html.EscapeString(strings.TrimSpace(data.UserAgent)) + "<br/>" +
		"<strong>Time (UTC):</strong> " + html.EscapeString(strings.TrimSpace(data.Timestamp)) +
		"</div>" +
		"<p style=\"margin:0 0 10px;font-size:13px;color:#6b7280;\">If this was not you, reset your password and secure your account immediately.</p>" +
		actionBlock +
		footerBlock(b) +
		"</div></body></html>"
}
