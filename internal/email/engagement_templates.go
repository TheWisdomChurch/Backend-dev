package email

import (
	"html"
	"strings"
)

type PastoralCareConfirmationTemplateData struct {
	Branding      Branding
	RecipientName string
	ReferenceID   string
	EventType     string
	EventDate     string
}

type GivingIntentConfirmationTemplateData struct {
	Branding      Branding
	RecipientName string
	ReferenceID   string
	Title         string
}

type WorkforceConfirmationTemplateData struct {
	Branding      Branding
	RecipientName string
	ReferenceID   string
	Department    string
	StatusLabel   string
}

func RenderPastoralCareConfirmationEmail(data PastoralCareConfirmationTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := safeName(data.RecipientName)
	ref := html.EscapeString(strings.TrimSpace(data.ReferenceID))
	eventType := html.EscapeString(strings.TrimSpace(data.EventType))
	eventDate := html.EscapeString(strings.TrimSpace(data.EventDate))

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:16px;padding:28px;\">" +
		renderLogoBlock(b) +
		"<h2 style=\"margin:0 0 10px;font-size:22px;\">Pastoral care request received</h2>" +
		"<p style=\"margin:0 0 14px;color:#334155;\">Hello " + name + ", we have received your request and our pastoral team will contact you shortly.</p>" +
		"<div style=\"background:#f8fafc;border-radius:12px;padding:14px;margin:0 0 14px;\">" +
		"<p style=\"margin:0 0 6px;font-size:13px;color:#475569;\"><strong>Reference:</strong> " + ref + "</p>" +
		"<p style=\"margin:0 0 6px;font-size:13px;color:#475569;\"><strong>Request type:</strong> " + eventType + "</p>" +
		"<p style=\"margin:0;font-size:13px;color:#475569;\"><strong>Date:</strong> " + eventDate + "</p>" +
		"</div>" +
		footerBlock(b) +
		"</div></body></html>"
}

func RenderGivingIntentConfirmationEmail(data GivingIntentConfirmationTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := safeName(data.RecipientName)
	ref := html.EscapeString(strings.TrimSpace(data.ReferenceID))
	title := html.EscapeString(strings.TrimSpace(data.Title))

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:16px;padding:28px;\">" +
		renderLogoBlock(b) +
		"<h2 style=\"margin:0 0 10px;font-size:22px;\">Giving request received</h2>" +
		"<p style=\"margin:0 0 14px;color:#334155;\">Hello " + name + ", thank you for your willingness to give. Our team will guide you with the next steps.</p>" +
		"<div style=\"background:#f8fafc;border-radius:12px;padding:14px;margin:0 0 14px;\">" +
		"<p style=\"margin:0 0 6px;font-size:13px;color:#475569;\"><strong>Reference:</strong> " + ref + "</p>" +
		"<p style=\"margin:0;font-size:13px;color:#475569;\"><strong>Category:</strong> " + title + "</p>" +
		"</div>" +
		footerBlock(b) +
		"</div></body></html>"
}

func RenderWorkforceConfirmationEmail(data WorkforceConfirmationTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := safeName(data.RecipientName)
	ref := html.EscapeString(strings.TrimSpace(data.ReferenceID))
	department := html.EscapeString(strings.TrimSpace(data.Department))
	statusLabel := html.EscapeString(strings.TrimSpace(data.StatusLabel))

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:16px;padding:28px;\">" +
		renderLogoBlock(b) +
		"<h2 style=\"margin:0 0 10px;font-size:22px;\">Workforce registration received</h2>" +
		"<p style=\"margin:0 0 14px;color:#334155;\">Hello " + name + ", your workforce registration has been received and recorded.</p>" +
		"<div style=\"background:#f8fafc;border-radius:12px;padding:14px;margin:0 0 14px;\">" +
		"<p style=\"margin:0 0 6px;font-size:13px;color:#475569;\"><strong>Reference:</strong> " + ref + "</p>" +
		"<p style=\"margin:0 0 6px;font-size:13px;color:#475569;\"><strong>Department:</strong> " + department + "</p>" +
		"<p style=\"margin:0;font-size:13px;color:#475569;\"><strong>Status:</strong> " + statusLabel + "</p>" +
		"</div>" +
		footerBlock(b) +
		"</div></body></html>"
}

func safeName(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "there"
	}
	return html.EscapeString(clean)
}
