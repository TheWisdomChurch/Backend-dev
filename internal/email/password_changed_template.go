package email

import "fmt"

type PasswordChangedTemplateData struct {
	Branding  Branding
	Email     string
	Timestamp string
	LoginURL  string
}

func RenderPasswordChangedEmail(data PasswordChangedTemplateData) string {
	app := data.Branding.AppName
	if app == "" {
		app = "Wisdom House"
	}
	loginURL := data.LoginURL
	if loginURL == "" && data.Branding.FrontendURL != "" {
		loginURL = data.Branding.FrontendURL
	}

	return fmt.Sprintf(`
<html>
  <body style="font-family: Arial, sans-serif; background:#f8f9fb; padding:20px; color:#111;">
    <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:auto;background:#fff;border:1px solid #e5e7eb;border-radius:8px;padding:24px;">
      <tr>
        <td style="font-size:18px;font-weight:bold;color:#111;">Your %s password was changed</td>
      </tr>
      <tr><td style="padding-top:12px;font-size:14px;line-height:1.6;color:#111;">
        Email: %s<br/>
        Time (UTC): %s
      </td></tr>
      <tr><td style="padding-top:16px;font-size:13px;color:#6b7280;">
        If you did not make this change, please reset your password immediately.
      </td></tr>
      %s
    </table>
  </body>
</html>`,
		app,
		data.Email,
		data.Timestamp,
		func() string {
			if loginURL == "" {
				return ""
			}
			return fmt.Sprintf(`<tr><td style="padding-top:16px;">
        <a href="%s" style="display:inline-block;padding:12px 16px;background:#111827;color:#fff;text-decoration:none;border-radius:6px;font-size:14px;">Return to login</a>
      </td></tr>`, loginURL)
		}(),
	)
}
