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

	fontStack = "-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif"
)

// renderEmailShell wraps bodyHTML in the standard chrome: page background,
// sharp-cornered card, a thin top rule (topRuleColor — colorAccent for
// ordinary mail, colorDanger for security alerts), the brand header, and the
// footer. Every Render* function in this package should return
// renderEmailShell(...) rather than building its own <html> wrapper.
func renderEmailShell(b Branding, topRuleColor string, bodyHTML string) string {
	b = normalizeBranding(b)
	if strings.TrimSpace(topRuleColor) == "" {
		topRuleColor = colorAccent
	}

	return "<!DOCTYPE html>" +
		"<html><body style=\"margin:0;padding:0;background:" + colorGround + ";font-family:" + fontStack + ";\">" +
		"<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"background:" + colorGround + ";\"><tr><td align=\"center\" style=\"padding:40px 16px;\">" +
		"<table role=\"presentation\" width=\"600\" cellpadding=\"0\" cellspacing=\"0\" style=\"width:600px;max-width:600px;background:" + colorPaper + ";border:1px solid " + colorLine + ";\">" +
		"<tr><td style=\"height:3px;line-height:3px;font-size:0;background:" + topRuleColor + ";\">&nbsp;</td></tr>" +
		renderHeaderBlock(b) +
		bodyHTML +
		renderFooterBlock(b) +
		"</table>" +
		"</td></tr></table>" +
		"</body></html>"
}

// renderHeaderBlock renders the brand lockup: logo on the left, a vertical
// hairline divider, then "The" / "Wisdom Church" stacked with the tagline
// beneath — followed by a full-width hairline separating the header from
// body content.
func renderHeaderBlock(b Branding) string {
	b = normalizeBranding(b)
	logoURL := brandLogoURL(b)

	logoCell := "<div style=\"width:56px;height:56px;background:" + colorInk + ";font-family:" + fontStack + ";font-size:22px;font-weight:800;line-height:56px;text-align:center;color:" + colorAccent + ";\">" + firstLetter(b.AppName) + "</div>"
	if logoURL != "" {
		logoCell = "<img src=\"" + html.EscapeString(logoURL) + "\" width=\"56\" height=\"56\" alt=\"" + html.EscapeString(b.AppName) + "\" style=\"display:block;width:56px;height:56px;border-radius:14px;object-fit:cover;\">"
	}

	tagline := strings.TrimSpace(b.Tagline())

	return "<tr><td style=\"padding:36px 40px 28px;\">" +
		"<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr>" +
		"<td style=\"width:56px;vertical-align:middle;\">" + logoCell + "</td>" +
		"<td style=\"width:1px;padding:0 10px;\">" +
		"<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr><td width=\"1\" height=\"52\" style=\"width:1px;font-size:0;line-height:0;background:" + colorLine + ";\">&nbsp;</td></tr></table>" +
		"</td>" +
		"<td style=\"vertical-align:middle;font-family:" + fontStack + ";\">" +
		"<div style=\"font-size:13px;font-weight:400;color:" + colorMuted + ";line-height:1.3;\">The</div>" +
		"<div style=\"font-size:18px;font-weight:800;letter-spacing:-.01em;color:" + colorInk + ";line-height:1.25;\">" + html.EscapeString(strings.TrimPrefix(strings.TrimSpace(b.AppName), "The ")) + "</div>" +
		conditionalTagline(tagline) +
		"</td>" +
		"</tr></table>" +
		"</td></tr>" +
		"<tr><td style=\"padding:0 40px;\"><div style=\"border-top:1px solid " + colorLine + ";\"></div></td></tr>"
}

func conditionalTagline(tagline string) string {
	if tagline == "" {
		return ""
	}
	return "<div style=\"font-size:10.5px;font-style:italic;font-weight:500;color:" + colorAccent + ";letter-spacing:.01em;margin-top:5px;\">" + html.EscapeString(tagline) + "</div>"
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

	return "<tr><td style=\"padding:0 40px;\"><div style=\"border-top:1px solid " + colorLine + ";\"></div></td></tr>" +
		"<tr><td style=\"padding:24px 40px 32px;font-family:" + fontStack + ";\">" +
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

	separator := "<span style=\"color:" + colorLine + ";padding:0 7px;\">&middot;</span>"
	return "<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr><td style=\"font-family:" + fontStack + ";font-size:11px;letter-spacing:.04em;\">" +
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
	return "<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr>" +
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
	return "<table role=\"presentation\" cellpadding=\"0\" cellspacing=\"0\"><tr>" +
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
	return "<div style=\"font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:" + color + ";margin-bottom:12px;\">" + html.EscapeString(text) + "</div>"
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
		tds.WriteString("<td style=\"width:" + strconv.FormatFloat(width, 'f', 2, 64) + "%;padding:18px " + padRight + " 18px " + padLeft + ";font-family:" + fontStack + ";" + borderRight + "\">")
		tds.WriteString("<div style=\"font-size:10px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:" + colorFaint + ";margin-bottom:6px;\">" + html.EscapeString(it.Label) + "</div>")
		tds.WriteString("<div style=\"font-size:14px;color:" + colorInk + ";\">" + html.EscapeString(it.Value) + "</div>")
		tds.WriteString("</td>")
	}

	return "<tr><td style=\"padding:16px 40px 0;\">" +
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
	return "<tr><td style=\"padding:36px 40px 8px;font-family:" + fontStack + ";\">"
}

func renderBodyClose() string {
	return "</td></tr>"
}

// renderActionRow wraps one or two buttons with the shell's standard spacing.
func renderActionRow(buttonsHTML string) string {
	if strings.TrimSpace(buttonsHTML) == "" {
		return ""
	}
	return "<tr><td style=\"padding:28px 40px 40px;\">" + buttonsHTML + "</td></tr>"
}
