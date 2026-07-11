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

	body := renderBodyOpen() +
		renderEyebrow("Action recommended", colorDanger) +
		renderHeading(b.AppName+" account security") +
		renderParagraph("We detected activity that needs your review.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "Reason", Value: strings.TrimSpace(data.Reason)},
			{Label: "IP", Value: strings.TrimSpace(data.IP)},
			{Label: "Time (UTC)", Value: strings.TrimSpace(data.Timestamp)},
		}) +
		"<tr><td style=\"padding:20px 40px 0;font-family:" + fontStack + ";\">" +
		"<p style=\"margin:0;font-size:14px;color:" + colorBody + ";\"><strong style=\"color:" + colorInk + ";\">Email:</strong> " + html.EscapeString(strings.TrimSpace(data.Email)) + "</p>" +
		"<p style=\"margin:8px 0 0;font-size:14px;color:" + colorBody + ";\"><strong style=\"color:" + colorInk + ";\">Browser:</strong> " + html.EscapeString(strings.TrimSpace(data.UserAgent)) + "</p>" +
		"</td></tr>" +
		"<tr><td style=\"padding:16px 40px 0;font-family:" + fontStack + ";\">" +
		"<p style=\"margin:0;font-size:13px;color:" + colorFaint + ";\">If this was not you, reset your password and secure your account immediately.</p>" +
		"</td></tr>" +
		renderActionRow(renderButton("Review devices", data.ManageURL, "", ""))

	return renderEmailShell(b, colorDanger, body)
}
