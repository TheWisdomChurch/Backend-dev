package email

import (
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
	if loginURL == "" {
		loginURL = strings.TrimSpace(data.Branding.FrontendURL)
	}

	body := renderBodyOpen() +
		renderEyebrow("Password changed", "") +
		renderHeading("Your "+b.AppName+" password was changed") +
		renderParagraph("This is a confirmation that the password for your account was changed.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "Email", Value: strings.TrimSpace(data.Email)},
			{Label: "Time (UTC)", Value: strings.TrimSpace(data.Timestamp)},
		}) +
		"<tr><td style=\"padding:16px 40px 0;font-family:" + fontStack + ";\">" +
		"<p style=\"margin:0;font-size:13px;color:" + colorFaint + ";\">If you did not make this change, please reset your password immediately.</p>" +
		"</td></tr>" +
		renderActionRow(renderButton("Return to login", loginURL, "", ""))

	return renderEmailShell(b, "", body)
}
