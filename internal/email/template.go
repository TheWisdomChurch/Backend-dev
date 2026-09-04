package email

import (
	"html"
	"strings"

	"wisdomHouse-backend/internal/models"
)

type NotificationTemplateData struct {
	Branding       Branding
	Title          string
	Message        string
	Event          *models.Event
	RecipientName  *string
	UnsubscribeURL string

	// ActionURL / ActionLabel render a real call-to-action button under the
	// message instead of leaving a bare URL in the body text. ActionLabel
	// defaults to "Open" when only the URL is set.
	ActionURL   string
	ActionLabel string

	// Internal marks a staff-only operational notification: the "you
	// subscribed to updates / unsubscribe" footer is replaced with a quiet
	// internal-notice line, since staff never opted in and can't opt out.
	Internal bool
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

	footerHTML := renderSubscriptionFooter(b.AppName, data.UnsubscribeURL)
	if data.Internal {
		footerHTML = renderInternalNotice(b.AppName)
	}

	actionRow := ""
	if url := strings.TrimSpace(data.ActionURL); url != "" {
		label := strings.TrimSpace(data.ActionLabel)
		if label == "" {
			label = "Open"
		}
		actionRow = renderActionRow(renderButton(label, url, "", ""))
	}

	body := renderBodyOpen() +
		"<p style=\"margin:0 0 4px;font-size:15px;color:" + colorBody + ";\">" + greeting + "</p>" +
		renderHeading(title) +
		renderParagraph(safeMessage) +
		eventBlock +
		renderBodyClose() +
		actionRow +
		"<tr><td class=\"wc-content-pad\" style=\"padding:0 48px 8px;font-family:" + fontStack + ";\">" +
		footerHTML +
		"</td></tr>"

	preheader := strings.TrimSpace(data.Message)
	if len(preheader) > 140 {
		preheader = strings.TrimSpace(preheader[:140])
	}
	return renderEmailShellWithPreheader(b, "", preheader, body)
}

// renderInternalNotice is the footer line for staff-only operational mail —
// no subscription language, no unsubscribe link.
func renderInternalNotice(appName string) string {
	return "<p style=\"margin:24px 0 0;font-size:12px;line-height:1.6;color:" + colorFaint + ";\">Internal notification for " + html.EscapeString(appName) + " administrators.</p>"
}

func RenderNotificationText(data NotificationTemplateData) string {
	var out strings.Builder
	if data.RecipientName != nil && strings.TrimSpace(*data.RecipientName) != "" {
		out.WriteString("Hello " + strings.TrimSpace(*data.RecipientName) + ",\n\n")
	} else {
		out.WriteString("Hello,\n\n")
	}
	out.WriteString(strings.TrimSpace(data.Title) + "\n\n")
	out.WriteString(strings.TrimSpace(data.Message))
	if data.Event != nil {
		out.WriteString("\n\nEvent: " + data.Event.Title + "\nDate: " + data.Event.Date + "\nTime: " + data.Event.Time + "\nLocation: " + data.Event.Location)
	}
	if url := strings.TrimSpace(data.ActionURL); url != "" {
		label := strings.TrimSpace(data.ActionLabel)
		if label == "" {
			label = "Open"
		}
		out.WriteString("\n\n" + label + ": " + url)
	}
	if data.Internal {
		appName := strings.TrimSpace(data.Branding.AppName)
		if appName == "" {
			appName = "The Wisdom Church"
		}
		out.WriteString("\n\nInternal notification for " + appName + " administrators.")
	} else {
		out.WriteString("\n\nYou received this because you subscribed to updates.")
		if url := strings.TrimSpace(data.UnsubscribeURL); url != "" {
			out.WriteString("\nUnsubscribe: " + url)
		}
	}
	return out.String()
}

func renderSubscriptionFooter(appName, unsubscribeURL string) string {
	footer := "<p style=\"margin:24px 0 0;font-size:12px;line-height:1.6;color:" + colorFaint + ";\">You received this because you subscribed to " + html.EscapeString(appName) + " updates."
	if url := strings.TrimSpace(unsubscribeURL); url != "" {
		footer += " <a href=\"" + html.EscapeString(url) + "\" style=\"color:" + colorMuted + ";text-decoration:underline;\">Unsubscribe</a>."
	}
	return footer + "</p>"
}
