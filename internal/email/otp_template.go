// internal/email/otp.go
package email

import (
	"fmt"
	"html"
	"strings"
	"time"
)

type OTPTemplateData struct {
	Branding     Branding
	Code         string
	Purpose      string
	ExpiresAt    time.Time
	ActionURL    string
	ActionLabel  string
	HeroImageURL string
}

// RenderOTPEmail renders a professional OTP / verification email.
func RenderOTPEmail(data OTPTemplateData) string {
	b := normalizeBranding(data.Branding)

	code := strings.TrimSpace(data.Code)
	safeCode := html.EscapeString(code)

	purpose := strings.TrimSpace(data.Purpose)
	purposeLine := "Use the verification code below to complete your request."
	if purpose != "" {
		purposeLine = fmt.Sprintf(
			"Use the verification code below to complete %s.",
			html.EscapeString(purpose),
		)
	}

	expiresAt := data.ExpiresAt
	expiresText := expiresAt.Format("Mon, 02 Jan 2006 15:04 MST")

	logoBlock := renderLogoBlock(b)
	heroBlock := renderHeroImageBlock(
		data.HeroImageURL,
		"Verification code",
	)

	actionURL := strings.TrimSpace(data.ActionURL)
	actionLabel := strings.TrimSpace(data.ActionLabel)
	if actionLabel == "" {
		actionLabel = "Verify code"
	}

	actionBlock := ""
	if actionURL != "" {
		actionBlock = `<table role="presentation" style="margin:20px 0 0;">
  <tr>
    <td style="border-radius:999px;background:#1d4ed8;">
      <a href="` + html.EscapeString(actionURL) + `" 
         style="display:inline-block;padding:12px 22px;font-size:14px;font-weight:600;
                color:#ffffff;text-decoration:none;border-radius:999px;">
        ` + html.EscapeString(actionLabel) + `
      </a>
    </td>
  </tr>
</table>`
	}

	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>` + html.EscapeString(b.AppName) + ` • Verification code</title>
  <meta name="viewport" content="width=device-width,initial-scale=1" />
</head>
<body style="margin:0;padding:0;background:#0f172a;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#0f172a;padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:560px;background:#020617;border-radius:20px;border:1px solid #1f2937;overflow:hidden;">
          <tr>
            <td style="padding:24px 24px 12px 24px;background:linear-gradient(135deg,#0f172a,#020617);">
              ` + logoBlock + `
            </td>
          </tr>
          <tr>
            <td style="padding:0 24px 8px 24px;">
              ` + heroBlock + `
            </td>
          </tr>
          <tr>
            <td style="padding:0 24px 4px 24px;">
              <h1 style="margin:0;font-size:22px;line-height:1.4;color:#e5e7eb;">
                Verify your email
              </h1>
            </td>
          </tr>
          <tr>
            <td style="padding:4px 24px 14px 24px;">
              <p style="margin:0;font-size:14px;line-height:1.6;color:#9ca3af;">
                ` + purposeLine + `
              </p>
            </td>
          </tr>
          <tr>
            <td style="padding:6px 24px 4px 24px;">
              <table role="presentation" cellspacing="0" cellpadding="0" style="width:100%;border-radius:16px;background:rgba(15,23,42,0.85);border:1px solid #1f2937;">
                <tr>
                  <td align="center" style="padding:18px 16px 10px 16px;">
                    <div style="font-size:30px;letter-spacing:8px;font-weight:700;color:#f9fafb;">
                      ` + safeCode + `
                    </div>
                  </td>
                </tr>
                <tr>
                  <td align="center" style="padding:0 16px 16px 16px;">
                    <p style="margin:8px 0 0 0;font-size:12px;color:#9ca3af;">
                      This code will expire at
                      <span style="color:#e5e7eb;">` + html.EscapeString(expiresText) + `</span>.
                    </p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding:0 24px 4px 24px;">
              ` + actionBlock + `
            </td>
          </tr>
          <tr>
            <td style="padding:12px 24px 8px 24px;">
              <p style="margin:0;font-size:12px;line-height:1.6;color:#6b7280;">
                If you did not request this code, you can safely ignore this email. Your account remains secure.
              </p>
            </td>
          </tr>
          <tr>
            <td style="padding:8px 24px 20px 24px;">
              ` + footerBlock(b) + `
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}
