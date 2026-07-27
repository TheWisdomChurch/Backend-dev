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

type PrayerRequestNotificationTemplateData struct {
	Branding     Branding
	ReferenceID  string
	Category     string
	SubmittedAt  string
	AdminViewURL string
}

// RenderPrayerRequestNotificationEmail intentionally excludes the prayer body
// and submitter PII. Email is a notification channel, not the pastoral record.
func RenderPrayerRequestNotificationEmail(data PrayerRequestNotificationTemplateData) string {
	b := normalizeBranding(data.Branding)
	action := ""
	if strings.TrimSpace(data.AdminViewURL) != "" {
		action = renderActionRow(renderButton("Review securely", data.AdminViewURL, "", ""))
	}
	body := renderBodyOpen() +
		renderHeading("New prayer request received") +
		renderParagraph("A new confidential prayer request is ready for review in the protected admin portal.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "Reference", Value: strings.TrimSpace(data.ReferenceID)},
			{Label: "Category", Value: strings.TrimSpace(data.Category)},
			{Label: "Submitted", Value: strings.TrimSpace(data.SubmittedAt)},
		}) + action
	return renderEmailShell(b, "", body)
}

func RenderPastoralCareConfirmationEmail(data PastoralCareConfirmationTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := safeName(data.RecipientName)

	body := renderBodyOpen() +
		renderHeading("Pastoral care request received") +
		renderParagraph("Hello "+name+", we have received your request and our pastoral team will contact you shortly.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "Reference", Value: strings.TrimSpace(data.ReferenceID)},
			{Label: "Request type", Value: strings.TrimSpace(data.EventType)},
			{Label: "Date", Value: strings.TrimSpace(data.EventDate)},
		})

	return renderEmailShell(b, "", body)
}

func RenderGivingIntentConfirmationEmail(data GivingIntentConfirmationTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := safeName(data.RecipientName)

	body := renderBodyOpen() +
		renderHeading("Giving request received") +
		renderParagraph("Hello "+name+", thank you for your willingness to give. Our team will guide you with the next steps.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "Reference", Value: strings.TrimSpace(data.ReferenceID)},
			{Label: "Category", Value: strings.TrimSpace(data.Title)},
		})

	return renderEmailShell(b, "", body)
}

func RenderWorkforceConfirmationEmail(data WorkforceConfirmationTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := safeName(data.RecipientName)

	body := renderBodyOpen() +
		renderHeading("Workforce registration received") +
		renderParagraph("Hello "+name+", your workforce registration has been received and recorded.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "Reference", Value: strings.TrimSpace(data.ReferenceID)},
			{Label: "Department", Value: strings.TrimSpace(data.Department)},
			{Label: "Status", Value: strings.TrimSpace(data.StatusLabel)},
		})

	return renderEmailShell(b, "", body)
}

func safeName(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "there"
	}
	return html.EscapeString(clean)
}
