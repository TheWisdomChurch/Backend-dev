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

	logoBlock := renderLogoBlock(b)

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:700px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;box-shadow:0 10px 30px rgba(15,23,42,0.06);\">" +
		logoBlock +
		"<p style=\"margin:0 0 12px;font-size:15px;color:#334155;\">Hello " + name + ",</p>" +
		"<h2 style=\"margin:0 0 14px;font-size:24px;color:#0b2447;\">Welcome to the " + dept + " team</h2>" +
		"<p style=\"margin:0 0 14px;font-size:15px;color:#334155;\">Your request to join the workforce has been approved by our Super Admin. We are excited to serve alongside you.</p>" +
		"<div style=\"margin:18px 0;padding:16px;background:#f8fafc;border-radius:12px;border:1px solid #e2e8f0;\">" +
		"<p style=\"margin:0 0 8px;font-size:14px;color:#0f172a;\"><strong>What to expect next:</strong></p>" +
		"<ul style=\"margin:0;padding-left:18px;font-size:14px;color:#475569;\">" +
		"<li>Our team lead will reach out with your first serving schedule.</li>" +
		"<li>We will share guidelines and resources to help you settle in.</li>" +
		"<li>Feel free to reply to this email if you have any questions.</li>" +
		"</ul></div>" +
		"<p style=\"margin:0 0 18px;font-size:15px;color:#334155;\">Thank you for stepping forward to serve. Together, we will make an impact.</p>" +
		"<div style=\"margin-top:20px;padding:14px 16px;background:#0f172a;border-radius:12px;color:#e2e8f0;\">" +
		"<p style=\"margin:0;font-size:14px;\">With gratitude,</p>" +
		"<p style=\"margin:4px 0 0;font-size:15px;font-weight:600;\">" + pastor + "</p>" +
		"<p style=\"margin:2px 0 0;font-size:13px;color:#cbd5e1;\">Resident Pastor, " + html.EscapeString(b.AppName) + "</p>" +
		"</div>" +
		footerBlock(b) +
		"</div></body></html>"
}
