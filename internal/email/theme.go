package email

import (
	"html"
	"strconv"
	"strings"
)

// Shared visual language for all outbound email. Sharp corners throughout (no
// border-radius anywhere), one restrained accent color, hairline rules in
// place of filled panels. Every template in this package builds on the
// helpers here instead of hand-rolling its own container/header/footer, so
// there is exactly one place that defines what a Wisdom Church email looks
// like.
const (
	colorInk    = "#0E1420" // primary text, headings, button backgrounds
	colorPaper  = "#FFFFFF" // card background
	colorGround = "#EEF0F3" // page background behind the card
	colorAccent = "#8A6D2F" // brass — the one accent color; used sparingly
	colorDanger = "#B4432C" // muted red — security/alert top rule only
	colorLine   = "#DADFE6" // hairline borders/dividers
	colorMuted  = "#5B6472" // secondary text
	colorFaint  = "#8A93A3" // footer/tertiary text
	colorBody   = "#3A414D" // body copy

	// Dark-mode counterparts, applied only under prefers-color-scheme:dark
	// (see the media block in renderEmailShellWithPreheader). Warm near-black
	// rather than pure #000 so the card still reads as a surface.
	colorInkDark    = "#F3F5F8"
	colorBodyDark   = "#C6CCD6"
	colorGroundDark = "#0B0F16"
	colorPaperDark  = "#151B25"
	colorLineDark   = "#2B3440"
	colorAccentDark = "#C6A667" // brass, lightened for contrast on dark

	fontStack = "-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif"
)

// renderEmailShell wraps bodyHTML in the standard chrome: page background,
// sharp-cornered card, a thin top rule (topRuleColor — colorAccent for
// ordinary mail, colorDanger for security alerts), the brand header, and the
// footer. Every Render* function in this package should return
// renderEmailShell(...) rather than building its own <html> wrapper.
func renderEmailShell(b Branding, topRuleColor string, bodyHTML string) string {
	return renderEmailShellWithPreheader(b, topRuleColor, "", bodyHTML)
}

// renderEmailShellWithPreheader is renderEmailShell plus a hidden preheader:
// the short line email clients show as the inbox preview next to the subject.
// Without it, clients fall back to scraping the first visible text (often a
// bare "Hello," or a fragment of markup). Pass "" to omit.
func renderEmailShellWithPreheader(b Branding, topRuleColor string, preheader string, bodyHTML string) string {
	b = normalizeBranding(b)
	if strings.TrimSpace(topRuleColor) == "" {
		topRuleColor = colorAccent
	}

	bodyHTML = addResponsiveContentClasses(bodyHTML)
	responsiveCSS := "<style>" +
		":root{color-scheme:light dark;supported-color-schemes:light dark}" +
		"body{margin:0!important;padding:0!important;-webkit-text-size-adjust:100%;-ms-text-size-adjust:100%}" +
		".wc-frame{width:100%!important;max-width:680px!important}.wc-content-pad{}.wc-button a{box-sizing:border-box}.wc-fluid-image{max-width:100%!important;height:auto!important}" +
		".wc-hero-cell{padding:0!important}.wc-hero{width:100%!important;max-width:100%!important;height:auto!important;border:0!important}" +
		"@media only screen and (max-width:700px){.wc-outer{padding:24px 14px!important}.wc-frame{max-width:100%!important}.wc-header{padding:30px 32px 24px!important}.wc-divider{padding-left:32px!important;padding-right:32px!important}.wc-content-pad,.wc-footer{padding-left:32px!important;padding-right:32px!important}}" +
		"@media only screen and (max-width:480px){.wc-outer{padding:10px 8px!important}.wc-frame{border-radius:14px!important}.wc-header{padding:24px 20px 20px!important}.wc-divider{padding-left:20px!important;padding-right:20px!important}.wc-content-pad,.wc-footer{padding-left:20px!important;padding-right:20px!important}.wc-button,.wc-button tbody,.wc-button tr,.wc-button td{display:block!important;width:100%!important}.wc-button a{display:block!important;width:100%!important;text-align:center!important;padding:15px 18px!important}.wc-info-cell{display:block!important;width:100%!important;border-right:0!important;border-bottom:1px solid " + colorLine + "!important;padding:14px 0!important}.wc-social a{display:inline-block!important;margin:0 10px 8px 0!important}.wc-social-separator{display:none!important}h1{font-size:25px!important;line-height:1.25!important}}" +
		// Dark mode: honoured by Apple Mail, iOS Mail and Outlook.com. Gmail
		// ignores prefers-color-scheme and applies its own partial inversion,
		// so this is best-effort — the light design is still the source of
		// truth. Text elements re-colour via descendant selectors from the
		// card, buttons and the brand tile are left alone.
		"@media (prefers-color-scheme:dark){" +
		"body,.wc-body-bg{background:" + colorGroundDark + "!important}" +
		".wc-outer{background:" + colorGroundDark + "!important}" +
		".wc-frame{background:" + colorPaperDark + "!important;border-color:" + colorLineDark + "!important}" +
		".wc-frame h1,.wc-frame p,.wc-frame td,.wc-frame div,.wc-frame span{color:" + colorBodyDark + "!important}" +
		".wc-frame strong{color:" + colorInkDark + "!important}" +
		".wc-divider div{border-color:" + colorLineDark + "!important}" +
		".wc-eyebrow,.wc-accent{color:" + colorAccentDark + "!important}" +
		".wc-hairline{border-color:" + colorLineDark + "!important}" +
		".wc-button td{background:#ECEEF2!important;border-color:#ECEEF2!important}.wc-button a{color:#151B25!important}" +
		"}" +
		"[data-ogsc] .wc-frame{background:" + colorPaperDark + "!important}" +
		"[data-ogsc] .wc-frame h1,[data-ogsc] .wc-frame p,[data-ogsc] .wc-frame td,[data-ogsc] .wc-frame div{color:" + colorBodyDark + "!important}" +
		" </style>"

	preheaderBlock := ""
	if p := strings.TrimSpace(preheader); p != "" {
		preheaderBlock = "<div style=\"display:none;overflow:hidden;line-height:1px;opacity:0;max-height:0;max-width:0;mso-hide:all;\">" +
			html.EscapeString(p) +
			strings.Repeat("&#847;&zwnj;&nbsp;", 60) +
			"</div>"
	}

	return "<!DOCTYPE html>" +
		"<html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"x-apple-disable-message-reformatting\">" +
		"<meta name=\"color-scheme\" content=\"light dark\"><meta name=\"supported-color-schemes\" content=\"light dark\">" + responsiveCSS + "</head>" +
		"<body class=\"wc-body-bg\" style=\"margin:0;padding:0;background:" + colorGround + ";color-scheme:light dark;font-family:" + fontStack + ";\">" +
		preheaderBlock +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"width:100%;background:" + colorGround + ";\"><tr><td class=\"wc-outer\" align=\"center\" style=\"padding:32px 20px;\">" +
		"<table class=\"wc-frame\" role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"width:100%;max-width:680px;background:" + colorPaper + ";border:1px solid " + colorLine + ";border-radius:20px;overflow:hidden;\">" +
		"<tr><td style=\"height:3px;line-height:3px;font-size:0;background:" + topRuleColor + ";\">&nbsp;</td></tr>" +
		renderHeaderBlock(b) +
		bodyHTML +
		renderFooterBlock(b) +
		"</table>" +
		"</td></tr></table>" +
		"</body></html>"
}

// EnsureResponsiveDocument applies the canonical responsive shell to custom,
// database, and remote-template HTML. Native package templates already carry
// the wc-frame marker and pass through unchanged.
func EnsureResponsiveDocument(b Branding, bodyHTML string) string {
	trimmed := strings.TrimSpace(bodyHTML)
	if trimmed == "" || strings.Contains(trimmed, "class=\"wc-frame\"") {
		return trimmed
	}
	lower := strings.ToLower(trimmed)
	if start := strings.Index(lower, "<body"); start >= 0 {
		if openEnd := strings.Index(lower[start:], ">"); openEnd >= 0 {
			contentStart := start + openEnd + 1
			if end := strings.LastIndex(lower, "</body>"); end > contentStart {
				trimmed = strings.TrimSpace(trimmed[contentStart:end])
			}
		}
	}
	return renderEmailShell(b, colorAccent, renderBodyOpen()+trimmed+renderBodyClose())
}

func addResponsiveContentClasses(body string) string {
	for _, vertical := range []string{"0", "8px", "14px", "16px", "20px", "24px", "28px", "32px", "36px"} {
		body = strings.ReplaceAll(body, "<td style=\"padding:"+vertical+" 40px", "<td class=\"wc-content-pad\" style=\"padding:"+vertical+" 40px")
	}
	return body
}

// renderHeaderBlock renders the brand lockup: logo on the left, a vertical
// hairline divider, then the church name. Generic classifications/taglines
// do not belong in the global shell: they made every message look like the
// same document regardless of its purpose. Template-specific context belongs
// in the message body via renderEyebrow.
func renderHeaderBlock(b Branding) string {
	b = normalizeBranding(b)
	logoURL := brandLogoURL(b)

	logoCell := "<div style=\"width:52px;height:52px;background:" + colorInk + ";font-family:" + fontStack + ";font-size:22px;font-weight:800;line-height:52px;text-align:center;color:" + colorAccent + ";border-radius:13px;\">" + firstLetter(b.AppName) + "</div>"
	if logoURL != "" {
		logoCell = "<img src=\"" + html.EscapeString(logoURL) + "\" width=\"52\" height=\"52\" alt=\"" + html.EscapeString(b.AppName) + "\" style=\"display:block;width:52px;height:52px;border-radius:13px;object-fit:cover;\">"
	}

	// One clean wordmark on a single line — no stacked "The" eyebrow, and the
	// full canonical AppName rendered as-is.
	wordmark := html.EscapeString(strings.TrimSpace(b.AppName))

	return "<tr><td class=\"wc-header\" style=\"padding:36px 48px 26px;\">" +
		"<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr>" +
		"<td style=\"width:52px;vertical-align:middle;\">" + logoCell + "</td>" +
		"<td style=\"width:1px;padding:0 16px;\">" +
		"<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr><td width=\"1\" height=\"30\" style=\"width:1px;font-size:0;line-height:0;background:" + colorLine + ";\">&nbsp;</td></tr></table>" +
		"</td>" +
		"<td style=\"vertical-align:middle;font-family:" + fontStack + ";\">" +
		"<div style=\"font-size:17px;font-weight:700;letter-spacing:.06em;text-transform:uppercase;color:" + colorInk + ";line-height:1.3;\">" + wordmark + "</div>" +
		"</td>" +
		"</tr></table>" +
		"</td></tr>" +
		"<tr><td class=\"wc-divider\" style=\"padding:0 48px;\"><div class=\"wc-hairline\" style=\"border-top:1px solid " + colorLine + ";\"></div></td></tr>"
}

func firstLetter(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "W"
	}
	return strings.ToUpper(string([]rune(s)[0]))
}

// renderFooterBlock renders the closing hairline, brand name, support
// contact, and — when configured — a row of social links.
func renderFooterBlock(b Branding) string {
	contactLine := ""
	if strings.TrimSpace(b.SupportEmail) != "" {
		contactLine = "<p style=\"margin:0 0 14px;font-size:12px;color:" + colorFaint + ";\">Need help? Contact <a href=\"mailto:" + html.EscapeString(b.SupportEmail) + "\" style=\"color:" + colorAccent + ";text-decoration:none;\">" + html.EscapeString(b.SupportEmail) + "</a></p>"
	}

	return "<tr><td class=\"wc-divider\" style=\"padding:0 48px;\"><div class=\"wc-hairline\" style=\"border-top:1px solid " + colorLine + ";\"></div></td></tr>" +
		"<tr><td class=\"wc-footer\" style=\"padding:26px 48px 36px;font-family:" + fontStack + ";\">" +
		"<p style=\"margin:0 0 4px;font-size:12px;color:" + colorFaint + ";\">" + html.EscapeString(b.AppName) + "</p>" +
		contactLine +
		renderSocialLinksRow(b.Social) +
		"</td></tr>"
}

// renderSocialLinksRow renders a quiet text-link row for configured social
// profiles, skipping any platform whose URL isn't set. Returns "" entirely
// when nothing is configured.
func renderSocialLinksRow(s SocialLinks) string {
	type link struct {
		label string
		url   string
	}
	links := []link{
		{"YouTube", s.YouTube},
		{"Instagram", s.Instagram},
		{"X", s.X},
		{"WhatsApp", s.WhatsApp},
		{"Facebook", s.Facebook},
		{"TikTok", s.TikTok},
	}

	var parts []string
	for _, l := range links {
		url := strings.TrimSpace(l.url)
		if url == "" {
			continue
		}
		parts = append(parts, "<a href=\""+html.EscapeString(url)+"\" style=\"color:"+colorMuted+";text-decoration:none;\">"+l.label+"</a>")
	}
	if len(parts) == 0 {
		return ""
	}

	separator := "<span class=\"wc-social-separator\" style=\"color:" + colorLine + ";padding:0 7px;\">&middot;</span>"
	return "<table class=\"wc-social\" role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr><td style=\"font-family:" + fontStack + ";font-size:11px;letter-spacing:.04em;\">" +
		strings.Join(parts, separator) +
		"</td></tr></table>"
}

// renderButton renders a solid, sharp-cornered call-to-action button. Pass
// "" for bg/fg to use the default ink-on-white treatment.
func renderButton(label, url, bg, fg string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}
	if strings.TrimSpace(bg) == "" {
		bg = colorInk
	}
	if strings.TrimSpace(fg) == "" {
		fg = colorPaper
	}
	return "<table class=\"wc-button\" role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr>" +
		"<td style=\"background:" + bg + ";\">" +
		"<a href=\"" + html.EscapeString(url) + "\" style=\"display:block;padding:14px 26px;font-family:" + fontStack + ";font-size:14px;font-weight:600;letter-spacing:.01em;color:" + fg + ";text-decoration:none;\">" + html.EscapeString(label) + "</a>" +
		"</td></tr></table>"
}

// renderOutlineButton renders a bordered, transparent-background button — the
// secondary action alongside a primary renderButton.
func renderOutlineButton(label, url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}
	return "<table class=\"wc-button\" role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr>" +
		"<td style=\"border:1px solid " + colorLine + ";\">" +
		"<a href=\"" + html.EscapeString(url) + "\" style=\"display:block;padding:13px 25px;font-family:" + fontStack + ";font-size:14px;font-weight:600;letter-spacing:.01em;color:" + colorInk + ";text-decoration:none;\">" + html.EscapeString(label) + "</a>" +
		"</td></tr></table>"
}

// renderTextLink renders a plain accent-colored link for lighter-weight
// secondary actions (e.g. "View full schedule").
func renderTextLink(label, url string) string {
	url = strings.TrimSpace(url)
	if url == "" || strings.TrimSpace(label) == "" {
		return ""
	}
	return "<a href=\"" + html.EscapeString(url) + "\" style=\"font-size:13px;color:" + colorAccent + ";text-decoration:none;\">" + html.EscapeString(label) + "</a>"
}

// renderEyebrow renders a small uppercase tracked label above a headline.
// Pass "" for color to use the default accent.
func renderEyebrow(text, color string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.TrimSpace(color) == "" {
		color = colorAccent
	}
	return "<div class=\"wc-eyebrow\" style=\"font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + color + ";margin-bottom:12px;\">" + html.EscapeString(text) + "</div>"
}

// infoItem is one label/value pair rendered by renderInfoGrid.
type infoItem struct {
	Label string
	Value string
}

// renderInfoGrid renders 2-4 label/value pairs as evenly-split columns
// separated by hairlines — used for security alert metadata, event
// date/venue, etc. Empty items are skipped.
func renderInfoGrid(items []infoItem) string {
	var filtered []infoItem
	for _, it := range items {
		if strings.TrimSpace(it.Value) != "" {
			filtered = append(filtered, it)
		}
	}
	if len(filtered) == 0 {
		return ""
	}

	width := 100.0 / float64(len(filtered))
	var tds strings.Builder
	for i, it := range filtered {
		borderRight := "border-right:1px solid " + colorLine + ";"
		padLeft := "16px"
		padRight := "16px"
		if i == 0 {
			padLeft = "0"
		}
		if i == len(filtered)-1 {
			borderRight = ""
			padRight = "0"
		}
		tds.WriteString("<td class=\"wc-info-cell\" style=\"width:" + strconv.FormatFloat(width, 'f', 2, 64) + "%;padding:18px " + padRight + " 18px " + padLeft + ";font-family:" + fontStack + ";" + borderRight + "\">")
		tds.WriteString("<div style=\"font-size:10px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:" + colorFaint + ";margin-bottom:6px;\">" + html.EscapeString(it.Label) + "</div>")
		tds.WriteString("<div style=\"font-size:14px;color:" + colorInk + ";\">" + html.EscapeString(it.Value) + "</div>")
		tds.WriteString("</td>")
	}

	return "<tr><td class=\"wc-content-pad\" style=\"padding:16px 48px 0;\">" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"border-top:1px solid " + colorLine + ";\"><tr>" +
		tds.String() +
		"</tr></table>" +
		"</td></tr>"
}

// renderCodeBlock renders a bordered, sharp-cornered display for a
// verification/registration code — the single consistent treatment used
// everywhere a code needs to stand out, replacing what used to be three
// different ad-hoc styles across templates.
func renderCodeBlock(label, code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if strings.TrimSpace(label) == "" {
		label = "Code"
	}
	return "<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\" style=\"margin:4px 0 0;\"><tr><td style=\"border:1px solid " + colorLine + ";padding:16px 22px;\">" +
		"<div style=\"font-family:" + fontStack + ";font-size:10px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + colorFaint + ";margin-bottom:6px;\">" + html.EscapeString(label) + "</div>" +
		"<div style=\"font-family:" + fontStack + ";font-size:24px;font-weight:700;letter-spacing:.08em;color:" + colorInk + ";font-variant-numeric:tabular-nums;\">" + html.EscapeString(code) + "</div>" +
		"</td></tr></table>"
}

// renderParagraph renders a standard body paragraph.
func renderParagraph(htmlContent string) string {
	if strings.TrimSpace(htmlContent) == "" {
		return ""
	}
	return "<p style=\"margin:0 0 16px;font-size:15px;line-height:1.65;color:" + colorBody + ";\">" + htmlContent + "</p>"
}

// renderHeading renders the main headline for a body block.
func renderHeading(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "<h1 style=\"margin:0 0 16px;font-size:23px;line-height:1.3;font-weight:700;letter-spacing:-.01em;color:" + colorInk + ";\">" + html.EscapeString(text) + "</h1>"
}

// renderBodyOpen/renderBodyClose wrap the main content cell inside the shell.
func renderBodyOpen() string {
	return "<tr><td class=\"wc-content-pad\" style=\"padding:38px 48px 10px;font-family:" + fontStack + ";\">"
}

func renderBodyClose() string {
	return "</td></tr>"
}

// renderActionRow wraps one or two buttons with the shell's standard spacing.
func renderActionRow(buttonsHTML string) string {
	if strings.TrimSpace(buttonsHTML) == "" {
		return ""
	}
	return "<tr><td class=\"wc-content-pad\" style=\"padding:30px 48px 44px;\">" + buttonsHTML + "</td></tr>"
}
