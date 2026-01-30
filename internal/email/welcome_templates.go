package email

import (
	"html"
	"strings"
)

type Branding struct {
	AppName              string
	LogoURL              string
	PublicURL            string
	FrontendURL          string
	SupportEmail         string
	PastorName           string
	AdminPortalURL       string
	TemplateAssetBaseURL string
}

type AdminWelcomeTemplateData struct {
	Branding      Branding
	RecipientName string
	Role          string
}

type SubscriberWelcomeTemplateData struct {
	Branding       Branding
	RecipientName  string
	UnsubscribeURL string
}

func RenderAdminWelcomeEmail(data AdminWelcomeTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := html.EscapeString(strings.TrimSpace(data.RecipientName))
	role := html.EscapeString(strings.TrimSpace(data.Role))
	if role == "" {
		role = "Admin"
	}
	portalURL := b.AdminPortalURL
	if portalURL == "" {
		base := b.FrontendURL
		if base == "" {
			base = b.PublicURL
		}
		portalURL = strings.TrimRight(base, "/") + "/admin"
	}

	logoBlock := renderLogoBlock(b)

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family: 'Segoe UI', Tahoma, Arial, sans-serif;line-height:1.6;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:16px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		"<h2 style=\"margin:0 0 12px;font-size:22px;\">Welcome to " + html.EscapeString(b.AppName) + " Administration</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">Hello " + name + ", your account has been created with <strong>" + role + "</strong> access.</p>" +
		"<div style=\"background:#f8fafc;border-radius:12px;padding:16px;margin:16px 0;\">" +
		"<p style=\"margin:0 0 8px;font-size:14px;\"><strong>Next steps:</strong></p>" +
		"<ul style=\"margin:0;padding-left:18px;font-size:14px;color:#475569;\">" +
		"<li>Review the admin dashboard for latest activity.</li>" +
		"<li>Ensure church programs and events are up to date.</li>" +
		"<li>Coordinate updates with the ministry team.</li>" +
		"</ul></div>" +
		"<a href=\"" + html.EscapeString(portalURL) + "\" style=\"display:inline-block;margin-top:8px;padding:12px 18px;background:#1d4ed8;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:600;\">Open Admin Portal</a>" +
		footerBlock(b) +
		"</div></body></html>"
}

func RenderSubscriberWelcomeEmail(data SubscriberWelcomeTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	greeting := "Dear friend,"
	if name != "" {
		greeting = "Dear " + html.EscapeString(name) + ","
	}

	logoBlock := renderLogoBlock(b)

	unsubscribeBlock := ""
	if strings.TrimSpace(data.UnsubscribeURL) != "" {
		unsubscribeBlock = "<p style=\"margin:24px 0 0;font-size:12px;color:#94a3b8;\">If you prefer not to receive these messages, you can <a href=\"" + html.EscapeString(data.UnsubscribeURL) + "\" style=\"color:#64748b;\">unsubscribe here</a>.</p>"
	}

	pastorName := strings.TrimSpace(b.PastorName)
	if pastorName == "" {
		pastorName = "Senior Pastor"
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family: 'Segoe UI', Tahoma, Arial, sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:680px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		"<h2 style=\"margin:0 0 12px;font-size:24px;color:#0b2447;\">Welcome to " + html.EscapeString(b.AppName) + "</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">" + greeting + "</p>" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">Thank you for subscribing. It is a joy to have you with us. We will keep you updated on upcoming programs, prayer meetings, and special moments in our community.</p>" +
		"<p style=\"margin:0 0 20px;font-size:15px;color:#334155;\">Please expect timely updates, encouragement, and invitations as we grow together in faith.</p>" +
		"<div style=\"margin-top:20px;padding:16px;background:#f8fafc;border-radius:12px;\">" +
		"<p style=\"margin:0;font-size:14px;color:#475569;\">With love and prayers,</p>" +
		"<p style=\"margin:4px 0 0;font-size:14px;font-weight:600;color:#1f2933;\">" + html.EscapeString(pastorName) + "</p>" +
		"</div>" +
		unsubscribeBlock +
		footerBlock(b) +
		"</div></body></html>"
}

type PasswordResetTemplateData struct {
	Branding      Branding
	RecipientName string
	ResetURL      string
	ExpiresAt     string
}

func RenderPasswordResetEmail(data PasswordResetTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}
	resetURL := html.EscapeString(strings.TrimSpace(data.ResetURL))
	expires := html.EscapeString(strings.TrimSpace(data.ExpiresAt))
	logoBlock := renderLogoBlock(b)

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f4f7fb;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		"<h2 style=\"margin:0 0 12px;font-size:22px;\">Reset your " + html.EscapeString(b.AppName) + " password</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">Hi " + name + ", we received a request to reset your password.</p>" +
		"<a href=\"" + resetURL + "\" style=\"display:inline-block;margin:8px 0;padding:12px 18px;background:#1d4ed8;color:#ffffff;text-decoration:none;border-radius:10px;font-weight:600;\">Verify and reset password</a>" +
		"<p style=\"margin:16px 0 0;font-size:13px;color:#6b7280;\">This link expires at " + expires + ".</p>" +
		"<p style=\"margin:12px 0 0;font-size:13px;color:#6b7280;\">If you did not request this, you can ignore this email.</p>" +
		footerBlock(b) +
		"</div></body></html>"
}

type LoginAlertTemplateData struct {
	Branding  Branding
	Email     string
	IP        string
	UserAgent string
	Timestamp string
}

func RenderLoginAlertEmail(data LoginAlertTemplateData) string {
	b := normalizeBranding(data.Branding)
	emailAddr := html.EscapeString(strings.TrimSpace(data.Email))
	ip := html.EscapeString(strings.TrimSpace(data.IP))
	ua := html.EscapeString(strings.TrimSpace(data.UserAgent))
	ts := html.EscapeString(strings.TrimSpace(data.Timestamp))
	logoBlock := renderLogoBlock(b)

	return "<!DOCTYPE html>" +
		"<html><body style=\"font-family:'Segoe UI',Tahoma,Arial,sans-serif;line-height:1.7;color:#0f172a;background:#f8fafc;padding:24px;\">" +
		"<div style=\"max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:32px;border:1px solid #e5e7eb;\">" +
		logoBlock +
		"<h2 style=\"margin:0 0 12px;font-size:22px;\">Security alert: failed login attempts</h2>" +
		"<p style=\"margin:0 0 16px;font-size:15px;color:#334155;\">We detected multiple failed login attempts on your account (" + emailAddr + ").</p>" +
		"<div style=\"background:#f8fafc;border-radius:12px;padding:16px;margin:16px 0;\">" +
		"<p style=\"margin:0 0 6px;font-size:14px;color:#475569;\"><strong>IP:</strong> " + ip + "</p>" +
		"<p style=\"margin:0 0 6px;font-size:14px;color:#475569;\"><strong>Device:</strong> " + ua + "</p>" +
		"<p style=\"margin:0;font-size:14px;color:#475569;\"><strong>Time:</strong> " + ts + "</p>" +
		"</div>" +
		"<p style=\"margin:0;font-size:14px;color:#334155;\">If this was not you, please reset your password immediately.</p>" +
		footerBlock(b) +
		"</div></body></html>"
}

func normalizeBranding(b Branding) Branding {
	if strings.TrimSpace(b.AppName) == "" {
		b.AppName = "Wisdom House"
	}
	if strings.TrimSpace(b.PublicURL) == "" {
		b.PublicURL = "http://localhost:8080"
	}
	if strings.TrimSpace(b.FrontendURL) == "" {
		b.FrontendURL = b.PublicURL
	}
	if strings.TrimSpace(b.TemplateAssetBaseURL) != "" {
		b.TemplateAssetBaseURL = strings.TrimRight(strings.TrimSpace(b.TemplateAssetBaseURL), "/")
	}
	return b
}

func renderLogoBlock(b Branding) string {
	if strings.TrimSpace(b.LogoURL) == "" {
		return "<div style=\"font-size:18px;font-weight:700;color:#0b2447;margin-bottom:16px;\">" + html.EscapeString(b.AppName) + "</div>"
	}
	return "<div style=\"margin-bottom:20px;\"><img src=\"" + html.EscapeString(b.LogoURL) + "\" alt=\"" + html.EscapeString(b.AppName) + " logo\" style=\"max-width:160px;height:auto;\"></div>"
}

func footerBlock(b Branding) string {
	contactLine := ""
	if strings.TrimSpace(b.SupportEmail) != "" {
		contactLine = "<p style=\"margin:16px 0 0;font-size:12px;color:#94a3b8;\">Need help? Contact us at <a href=\"mailto:" + html.EscapeString(b.SupportEmail) + "\" style=\"color:#64748b;\">" + html.EscapeString(b.SupportEmail) + "</a>.</p>"
	}
	return "<p style=\"margin:24px 0 0;font-size:12px;color:#94a3b8;\">" + html.EscapeString(b.AppName) + "</p>" + contactLine
}
