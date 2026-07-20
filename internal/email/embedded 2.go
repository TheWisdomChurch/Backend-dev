package email

import _ "embed"

// LogoBytes is the church's brand logo, embedded directly into the binary so
// it's servable without depending on external CDN upload/config. Referenced
// by brandLogoURL() as the fallback when APP_LOGO_URL isn't set, served over
// HTTP by the /assets/logo.webp route (see cmd/api/router.go).
//
//go:embed assets/logo.webp
var LogoBytes []byte

// LogoContentType is the MIME type for LogoBytes.
const LogoContentType = "image/webp"

// LogoAssetPath is the public URL path LogoBytes is served under.
const LogoAssetPath = "/assets/logo.webp"
