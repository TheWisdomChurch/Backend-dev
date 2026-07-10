package email

import (
	"html"
	"strings"
)

type WorkforceApprovalTemplateData struct {
	Branding      Branding
	RecipientName string
	Department    string
}

func RenderWorkforceApprovalEmail(data WorkforceApprovalTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "Beloved"
	} else {
		name = html.EscapeString(name)
	}

	dept := strings.TrimSpace(data.Department)
	if dept == "" {
		dept = "our ministry team"
	} else {
		dept = html.EscapeString(dept)
	}

	pastor := strings.TrimSpace(b.PastorName)
	if pastor == "" {
		pastor = "Resident Pastor"
	} else {
		pastor = html.EscapeString(pastor)
	}

	body := renderBodyOpen() +
		renderEyebrow("Application approved", "") +
		renderHeading("Welcome to the "+dept+" team") +
		"<p style=\"margin:0 0 16px;font-size:15px;line-height:1.65;color:" + colorBody + ";\">Hello " + name + ", your request to join the workforce has been approved by our Super Admin. We are excited to serve alongside you.</p>" +
		renderBodyClose() +
		"<tr><td style=\"padding:0 40px 8px;\">" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"border:1px solid " + colorLine + ";\"><tr><td style=\"padding:18px 20px;font-family:" + fontStack + ";\">" +
		"<div style=\"font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + colorMuted + ";margin-bottom:10px;\">What to expect next</div>" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\">" +
		"<tr><td style=\"padding:5px 0;font-size:14px;color:" + colorBody + ";line-height:1.5;\">&mdash; Our team lead will reach out with your first serving schedule.</td></tr>" +
		"<tr><td style=\"padding:5px 0;font-size:14px;color:" + colorBody + ";line-height:1.5;\">&mdash; We will share guidelines and resources to help you settle in.</td></tr>" +
		"<tr><td style=\"padding:5px 0;font-size:14px;color:" + colorBody + ";line-height:1.5;\">&mdash; Feel free to reply to this email if you have any questions.</td></tr>" +
		"</table></td></tr></table>" +
		"</td></tr>" +
		"<tr><td style=\"padding:20px 40px 8px;font-family:" + fontStack + ";\">" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:" + colorBody + ";\">Thank you for stepping forward to serve. Together, we will make an impact.</p>" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"background:" + colorInk + ";\"><tr><td style=\"padding:16px 18px;\">" +
		"<p style=\"margin:0;font-size:14px;color:" + colorPaper + ";\">With gratitude,</p>" +
		"<p style=\"margin:4px 0 0;font-size:15px;font-weight:600;color:" + colorPaper + ";\">" + pastor + "</p>" +
		"<p style=\"margin:2px 0 0;font-size:13px;color:" + colorAccent + ";\">Resident Pastor, " + html.EscapeString(b.AppName) + "</p>" +
		"</td></tr></table>" +
		"</td></tr>"

	return renderEmailShell(b, "", body)
}
