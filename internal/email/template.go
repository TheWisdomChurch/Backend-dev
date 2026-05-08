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
	safeTitle := html.EscapeString(strings.TrimSpace(data.Title))
	safeMessage := html.EscapeString(strings.TrimSpace(data.Message))
	safeMessage = strings.ReplaceAll(safeMessage, "\n", "<br>")

	greeting := "Hello,"
	if data.RecipientName != nil {
		name := strings.TrimSpace(*data.RecipientName)
		if name != "" {
			greeting = "Hello " + html.EscapeString(name) + ","
		}
	}

	var eventBlock strings.Builder
	if data.Event != nil {
		eventBlock.WriteString("<div style=\"margin-top:24px;padding:16px;border:1px solid #e5e5e5;border-radius:8px;\">")
		eventBlock.WriteString("<h3 style=\"margin:0 0 12px;font-size:18px;\">Event Details</h3>")
		eventBlock.WriteString("<p style=\"margin:0 0 6px;\"><strong>Title:</strong> " + html.EscapeString(data.Event.Title) + "</p>")
		eventBlock.WriteString("<p style=\"margin:0 0 6px;\"><strong>Date:</strong> " + html.EscapeString(data.Event.Date) + "</p>")
		eventBlock.WriteString("<p style=\"margin:0 0 6px;\"><strong>Time:</strong> " + html.EscapeString(data.Event.Time) + "</p>")
		eventBlock.WriteString("<p style=\"margin:0 0 6px;\"><strong>Location:</strong> " + html.EscapeString(data.Event.Location) + "</p>")
		if data.Event.RegisterLink != nil && strings.TrimSpace(*data.Event.RegisterLink) != "" {
			link := html.EscapeString(strings.TrimSpace(*data.Event.RegisterLink))
			eventBlock.WriteString("<div style=\"margin-top:12px;\"><a href=\"" + link + "\" style=\"display:inline-block;padding:10px 16px;background:#1f5eff;color:#ffffff;text-decoration:none;border-radius:6px;\">Register</a></div>")
		}
		eventBlock.WriteString("</div>")
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.6;color:#1f2933;background:#f7f9fc;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		renderLogoBlock(b) +
		"<p style=\"margin-top:0;font-size:16px;\">" + greeting + "</p>" +
		"<h2 style=\"margin:8px 0 12px;font-size:22px;color:#0b2447;\">" + safeTitle + "</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;\">" + safeMessage + "</p>" +
		eventBlock.String() +
		"<p style=\"margin-top:24px;font-size:13px;color:#6b7280;\">You are receiving this email because you subscribed to " + html.EscapeString(b.AppName) + " updates.</p>" +
		footerBlock(b) +
		"</div></body></html>"
}
