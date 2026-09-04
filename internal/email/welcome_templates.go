package email

import (
	"html"
	"net/url"
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
	AppTagline           string
	Social               SocialLinks
}

// Tagline returns the configured tagline, or the default if unset.
func (b Branding) Tagline() string {
	if t := strings.TrimSpace(b.AppTagline); t != "" {
		return t
	}
	return "Equipped. Empowered for Greatness"
}

// SocialLinks holds the church's social profile URLs shown in email footers.
// Any field left empty is simply omitted from the footer, never linked as a
// guess.
type SocialLinks struct {
	YouTube   string
	Instagram string
	X         string
	WhatsApp  string
	Facebook  string
	TikTok    string
}

// HasAny reports whether at least one social link is configured.
func (s SocialLinks) HasAny() bool {
	return strings.TrimSpace(s.YouTube) != "" ||
		strings.TrimSpace(s.Instagram) != "" ||
		strings.TrimSpace(s.X) != "" ||
		strings.TrimSpace(s.WhatsApp) != "" ||
		strings.TrimSpace(s.Facebook) != "" ||
		strings.TrimSpace(s.TikTok) != ""
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
	portalURL := adminPortalURL(b)

	body := renderBodyOpen() +
		renderEyebrow("Account created", "") +
		renderHeading("Welcome to the administration team") +
		renderParagraph("Hello "+name+", your account has been created with <strong style=\"color:"+colorInk+";\">"+role+"</strong> access.") +
		renderBodyClose() +
		"<tr><td style=\"padding:8px 40px 0;\">" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"border-top:1px solid " + colorLine + ";border-bottom:1px solid " + colorLine + ";\">" +
		"<tr><td style=\"padding:20px 0;font-family:" + fontStack + ";\">" +
		"<div style=\"font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + colorMuted + ";margin-bottom:10px;\">Next steps</div>" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\">" +
		"<tr><td style=\"padding:5px 0;font-size:14px;color:" + colorBody + ";line-height:1.5;\">&mdash; Review the dashboard for latest submissions and activity</td></tr>" +
		"<tr><td style=\"padding:5px 0;font-size:14px;color:" + colorBody + ";line-height:1.5;\">&mdash; Confirm upcoming events and programs are current</td></tr>" +
		"<tr><td style=\"padding:5px 0;font-size:14px;color:" + colorBody + ";line-height:1.5;\">&mdash; Coordinate outstanding approvals with the ministry team</td></tr>" +
		"</table></td></tr></table>" +
		"</td></tr>" +
		renderActionRow(renderButton("Open admin portal", portalURL, "", ""))

	return renderEmailShell(b, "", body)
}

type AdminApprovedTemplateData struct {
	Branding      Branding
	RecipientName string
	LoginURL      string
}

func RenderAdminApprovedEmail(data AdminApprovedTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	if name == "" {
		name = "there"
	} else {
		name = html.EscapeString(name)
	}

	loginURL := strings.TrimSpace(data.LoginURL)
	if loginURL == "" {
		loginURL = adminLoginURL(b)
	}

	body := renderBodyOpen() +
		renderEyebrow("Account approved", "") +
		renderHeading("Your admin account is approved") +
		renderParagraph("Hi "+name+", your admin account has been successfully created and approved. You can now log in with your credentials.") +
		renderBodyClose() +
		renderActionRow(renderButton("Visit website", loginURL, "", ""))

	return renderEmailShell(b, "", body)
}

func RenderSubscriberWelcomeEmail(data SubscriberWelcomeTemplateData) string {
	b := normalizeBranding(data.Branding)
	name := strings.TrimSpace(data.RecipientName)
	greeting := "Dear friend,"
	if name != "" {
		greeting = "Dear " + html.EscapeString(name) + ","
	}

	pastorName := strings.TrimSpace(b.PastorName)
	if pastorName == "" {
		pastorName = "Senior Pastor"
	}

	unsubscribeBlock := ""
	if strings.TrimSpace(data.UnsubscribeURL) != "" {
		unsubscribeBlock = "<p style=\"margin:20px 0 0;font-size:12px;color:" + colorFaint + ";\">If you prefer not to receive these messages, you can <a href=\"" + html.EscapeString(data.UnsubscribeURL) + "\" style=\"color:" + colorMuted + ";\">unsubscribe here</a>.</p>"
	}

	body := renderBodyOpen() +
		renderHeading("Welcome to "+b.AppName) +
		renderParagraph(greeting) +
		renderParagraph("Thank you for subscribing. It is a joy to have you with us. We will keep you updated on upcoming programs, prayer meetings, and special moments in our community.") +
		renderParagraph("Please expect timely updates, encouragement, and invitations as we grow together in faith.") +
		renderBodyClose() +
		"<tr><td style=\"padding:0 40px 8px;\">" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"border-top:1px solid " + colorLine + ";\">" +
		"<tr><td style=\"padding:18px 0;font-family:" + fontStack + ";\">" +
		"<p style=\"margin:0;font-size:14px;color:" + colorMuted + ";\">With love and prayers,</p>" +
		"<p style=\"margin:4px 0 0;font-size:14px;font-weight:600;color:" + colorInk + ";\">" + html.EscapeString(pastorName) + "</p>" +
		"</td></tr></table>" +
		unsubscribeBlock +
		"</td></tr>"

	return renderEmailShell(b, "", body)
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
	expires := html.EscapeString(strings.TrimSpace(data.ExpiresAt))

	body := renderBodyOpen() +
		renderEyebrow("Password reset requested", "") +
		renderHeading("Reset your "+b.AppName+" password") +
		renderParagraph("Hi "+name+", we received a request to reset your password.") +
		renderBodyClose() +
		"<tr><td style=\"padding:8px 40px 40px;\">" +
		renderButton("Verify and reset password", data.ResetURL, "", "") +
		"<p style=\"margin:16px 0 0;font-size:13px;color:" + colorFaint + ";\">This link expires at " + expires + ".</p>" +
		"<p style=\"margin:8px 0 0;font-size:13px;color:" + colorFaint + ";\">If you did not request this, you can ignore this email.</p>" +
		"</td></tr>"

	return renderEmailShell(b, "", body)
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

	body := renderBodyOpen() +
		renderEyebrow("Action recommended", colorDanger) +
		renderHeading("Multiple failed sign-in attempts") +
		renderParagraph("We detected repeated failed sign-in attempts on your account <strong style=\"color:"+colorInk+";\">"+emailAddr+"</strong>. If this was you, no action is needed. If not, reset your password now.") +
		renderBodyClose() +
		renderInfoGrid([]infoItem{
			{Label: "IP address", Value: strings.TrimSpace(data.IP)},
			{Label: "Device", Value: strings.TrimSpace(data.UserAgent)},
			{Label: "Time", Value: strings.TrimSpace(data.Timestamp)},
		}) +
		renderActionRow(renderButton("Reset password", adminLoginURL(b), "", ""))

	return renderEmailShell(b, colorDanger, body)
}

func normalizeBranding(b Branding) Branding {
	if strings.TrimSpace(b.AppName) == "" {
		b.AppName = "The Wisdom Church"
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

// brandLogoURL resolves the logo image URL for email templates. Prefers an
// explicitly configured APP_LOGO_URL (e.g. a CDN asset); otherwise falls back
// to this backend's own embedded logo, served at PublicURL+LogoAssetPath
// (see embedded.go and the /assets/logo.png route in cmd/api/router.go).
func brandLogoURL(b Branding) string {
	if logo := strings.TrimSpace(b.LogoURL); logo != "" {
		return logo
	}

	base := strings.TrimSpace(b.PublicURL)
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Path = LogoAssetPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func adminPortalURL(b Branding) string {
	portal := strings.TrimSpace(b.AdminPortalURL)
	if portal != "" {
		return portal
	}

	base := strings.TrimSpace(b.FrontendURL)
	if base == "" {
		base = strings.TrimSpace(b.PublicURL)
	}
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/admin"
}

func adminLoginURL(b Branding) string {
	portal := strings.TrimSpace(adminPortalURL(b))
	if portal == "" {
		return ""
	}
	u, err := url.Parse(portal)
	if err != nil {
		return ""
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(strings.ToLower(path), "/login") {
		return u.String()
	}
	if path == "" {
		u.Path = "/login"
	} else {
		u.Path = path + "/login"
	}
	return u.String()
}
