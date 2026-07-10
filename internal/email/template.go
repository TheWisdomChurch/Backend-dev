package email

import (
	"html"
	"strings"

	"wisdomHouse-backend/internal/models"
)

type NotificationTemplateData struct {
	Branding      Branding
	Title         string
	Message       string
	Event         *models.Event
	RecipientName *string
}

func RenderNotificationEmail(data NotificationTemplateData) string {
	b := normalizeBranding(data.Branding)
	title := strings.TrimSpace(data.Title)
	safeMessage := html.EscapeString(strings.TrimSpace(data.Message))
	safeMessage = strings.ReplaceAll(safeMessage, "\n", "<br>")

	greeting := "Hello,"
	if data.RecipientName != nil {
		name := strings.TrimSpace(*data.RecipientName)
		if name != "" {
			greeting = "Hello " + html.EscapeString(name) + ","
		}
	}

	eventBlock := ""
	if data.Event != nil {
		registerBlock := ""
		if data.Event.RegisterLink != nil && strings.TrimSpace(*data.Event.RegisterLink) != "" {
			registerBlock = "<div style=\"margin-top:14px;\">" + renderButton("Register", strings.TrimSpace(*data.Event.RegisterLink), "", "") + "</div>"
		}
		eventBlock = "<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"border:1px solid " + colorLine + ";margin-top:20px;\"><tr><td style=\"padding:18px 20px;font-family:" + fontStack + ";\">" +
			"<div style=\"font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + colorMuted + ";margin-bottom:10px;\">Event details</div>" +
			"<p style=\"margin:0 0 6px;font-size:14px;color:" + colorBody + ";\"><strong style=\"color:" + colorInk + ";\">Title:</strong> " + html.EscapeString(data.Event.Title) + "</p>" +
			"<p style=\"margin:0 0 6px;font-size:14px;color:" + colorBody + ";\"><strong style=\"color:" + colorInk + ";\">Date:</strong> " + html.EscapeString(data.Event.Date) + "</p>" +
			"<p style=\"margin:0 0 6px;font-size:14px;color:" + colorBody + ";\"><strong style=\"color:" + colorInk + ";\">Time:</strong> " + html.EscapeString(data.Event.Time) + "</p>" +
			"<p style=\"margin:0;font-size:14px;color:" + colorBody + ";\"><strong style=\"color:" + colorInk + ";\">Location:</strong> " + html.EscapeString(data.Event.Location) + "</p>" +
			registerBlock +
			"</td></tr></table>"
	}

	body := renderBodyOpen() +
		"<p style=\"margin:0 0 4px;font-size:15px;color:" + colorBody + ";\">" + greeting + "</p>" +
		renderHeading(title) +
		renderParagraph(safeMessage) +
		eventBlock +
		"<p style=\"margin:24px 0 0;font-size:13px;color:" + colorFaint + ";\">You are receiving this email because you subscribed to " + html.EscapeString(b.AppName) + " updates.</p>" +
		renderBodyClose()

	return renderEmailShell(b, "", body)
}
