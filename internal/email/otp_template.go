// internal/email/otp.go
package email

import (
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

// RenderOTPEmail renders a verification code email.
func RenderOTPEmail(data OTPTemplateData) string {
	b := normalizeBranding(data.Branding)

	code := strings.TrimSpace(data.Code)
	headline, purposeLine := otpPurposeCopy(strings.TrimSpace(data.Purpose))
	expiresText := data.ExpiresAt.Format("Mon, 02 Jan 2006 15:04 MST")

	actionLabel := strings.TrimSpace(data.ActionLabel)
	if actionLabel == "" {
		actionLabel = "Verify code"
	}

	actionBlock := ""
	if btn := renderButton(actionLabel, data.ActionURL, "", ""); btn != "" {
		actionBlock = "<div style=\"margin-top:20px;\">" + btn + "</div>"
	}

	body := renderBodyOpen() +
		renderEyebrow("Verification code", "") +
		renderHeading(headline) +
		renderParagraph(purposeLine) +
		renderBodyClose() +
		renderHeroImageBlock(data.HeroImageURL, "Verification code") +
		"<tr><td style=\"padding:8px 40px 28px;\">" +
		renderCodeBlock("Your code", code) +
		"<p style=\"margin:12px 0 0;font-size:12px;color:" + colorFaint + ";\">This code expires at " + expiresText + ".</p>" +
		actionBlock +
		"<p style=\"margin:16px 0 0;font-size:12px;line-height:1.6;color:" + colorFaint + ";\">If you did not request this code, you can safely ignore this email. Your account remains secure.</p>" +
		"</td></tr>"

	return renderEmailShell(b, "", body)
}

func otpPurposeCopy(purpose string) (headline string, line string) {
	p := strings.TrimSpace(purpose)
	if p == "" {
		return "Verification code", "Use the verification code below to complete your request."
	}
	if i := strings.Index(p, ":"); i >= 0 {
		p = p[:i]
	}
	switch p {
	case "password_reset":
		return "Reset your password", "Use the verification code below to reset your password."
	case "login":
		return "Approve sign-in", "Use the verification code below to approve your sign-in."
	default:
		return "Verification code", "Use the verification code below to complete your request."
	}
}
