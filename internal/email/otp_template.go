package email

import (
	"fmt"
	"html"
	"strings"
	"time"
)

type OTPTemplateData struct {
	Branding     Branding
	Code         string
	Purpose      string
	ExpiresAt    time.Time
	ActionURL    string
	ActionLabel  string
	HeroImageURL string
}

func RenderOTPEmail(data OTPTemplateData) string {
	b := normalizeBranding(data.Branding)
	safeCode := html.EscapeString(strings.TrimSpace(data.Code))
	purpose := strings.TrimSpace(data.Purpose)
	purposeLine := "Use the verification code below to complete your request."
	if purpose != "" {
		purposeLine = fmt.Sprintf("Use the verification code below to complete %s.", html.EscapeString(purpose))
	}

	expires := data.ExpiresAt.Format(time.RFC1123)
	logoBlock := renderLogoBlock(b)
	heroBlock := renderHeroImageBlock(data.HeroImageURL, "Verification code")
	actionURL := strings.TrimSpace(data.ActionURL)
	actionLabel := strings.TrimSpace(data.ActionLabel)
	if actionLabel == "" {
		actionLabel = "Verify code"
	}

	actionBlock := ""
	if actionURL != "" {
		actionBlock = "<a href=\"" + html.EscapeString(actionURL) + "\" style=\"display:inline-block;margin:14px 0 0;padding:12px 16px;background:#1d4ed8;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:600;\">"
		actionBlock += html.EscapeString(actionLabel) + "</a>"
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.6;color:#111827;background:#f9fafb;padding:24px;\">" +
		"<div style=\"max-width:560px;margin:0 auto;background:#ffffff;border-radius:14px;padding:28px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		heroBlock +
		"<h2 style=\"margin:0 0 12px;font-size:20px;color:#0f172a;\">Verify your email</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;\">" + purposeLine + "</p>" +
		"<div style=\"font-size:28px;letter-spacing:6px;font-weight:bold;color:#0b2447;\">" + safeCode + "</div>" +
		"<p style=\"margin:16px 0 0;font-size:13px;color:#6b7280;\">Expires: " + html.EscapeString(expires) + "</p>" +
		actionBlock +
		footerBlock(b) +
		"</div></body></html>"
}
