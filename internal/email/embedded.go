package email

import _ "embed"

// LogoBytes is the church's brand logo, embedded directly into the binary so
// it's servable without depending on external CDN upload/config. Referenced
// by brandLogoURL() as the fallback when APP_LOGO_URL isn't set, served over
// HTTP by the /assets/logo.png route (see cmd/api/router.go).
//
// PNG, not WebP: Gmail, Outlook and Yahoo do not render WebP in email, so a
// WebP logo shows as a broken image for most recipients.
//
//go:embed assets/logo.png
var LogoBytes []byte

// LogoContentType is the MIME type for LogoBytes.
const LogoContentType = "image/png"

// LogoAssetPath is the public URL path LogoBytes is served under.
const LogoAssetPath = "/assets/logo.png"
