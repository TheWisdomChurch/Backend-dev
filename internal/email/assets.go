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

func renderHeroImageBlock(url string, alt string) string {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return ""
	}
	altText := strings.TrimSpace(alt)
	if altText == "" {
		altText = "Email illustration"
	}
	return "<div style=\"margin:8px 0 18px;\"><img src=\"" + html.EscapeString(trimmed) + "\" alt=\"" + html.EscapeString(altText) + "\" style=\"width:100%;max-width:560px;height:auto;border-radius:12px;\"></div>"
}
