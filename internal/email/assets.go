package email

import (
	"html"
	"strings"
)

// TemplateAssetURL builds a URL for a template asset using a base CDN URL.
// Example: {base}/registration/hero.png
func TemplateAssetURL(b Branding, templateKey, fileName string) string {
	base := strings.TrimRight(strings.TrimSpace(b.TemplateAssetBaseURL), "/")
	if base == "" {
		return ""
	}
	tk := strings.Trim(strings.TrimSpace(templateKey), "/")
	fn := strings.TrimLeft(strings.TrimSpace(fileName), "/")
	if tk == "" || fn == "" {
		return ""
	}
	return base + "/" + tk + "/" + fn
}

// ResolveTemplateAssetURL returns an absolute URL for an asset.
// If keyOrURL is already absolute, it is returned as-is.
// If keyOrURL is a relative key, it is joined to TemplateAssetBaseURL.
func ResolveTemplateAssetURL(b Branding, keyOrURL string) string {
	raw := strings.TrimSpace(keyOrURL)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return raw
	}
	base := strings.TrimRight(strings.TrimSpace(b.TemplateAssetBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(raw, "/")
}

// renderHeroImageBlock renders a full table row containing a full-bleed hero
// image, or "" when no URL is given — callers can concatenate it directly at
// the table-row level alongside renderInfoGrid/renderActionRow/etc.
//
// The image runs edge to edge inside the card (no side padding, no border, no
// width cap): the card's border-radius + overflow:hidden clip it cleanly, and
// on a narrow phone it fills the width instead of shrinking to a bordered
// thumbnail inside 40px of padding.
func renderHeroImageBlock(url string, alt string) string {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return ""
	}
	altText := strings.TrimSpace(alt)
	if altText == "" {
		altText = "Email illustration"
	}
	return "<tr><td class=\"wc-hero-cell\" style=\"padding:0;\">" +
		"<img src=\"" + html.EscapeString(trimmed) + "\" alt=\"" + html.EscapeString(altText) + "\" class=\"wc-hero\" style=\"display:block;width:100%;max-width:100%;height:auto;border:0;margin:0;\">" +
		"</td></tr>"
}
