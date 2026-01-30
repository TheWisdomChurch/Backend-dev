package email

import "fmt"

// SecurityAlertTemplateData is the data fed into the security alert email.
type SecurityAlertTemplateData struct {
	Branding  Branding
	Email     string
	Reason    string
	IP        string
	UserAgent string
	Timestamp string
	ManageURL string
}

// RenderSecurityAlertEmail renders a simple HTML email notifying of a security event.
func RenderSecurityAlertEmail(data SecurityAlertTemplateData) string {
	app := data.Branding.AppName
	if app == "" {
		app = "Wisdom House"
	}

	return fmt.Sprintf(`
<html>
  <body style="font-family: Arial, sans-serif; background:#f8f9fb; padding:20px; color:#111;">
    <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:auto;background:#fff;border:1px solid #e5e7eb;border-radius:8px;padding:24px;">
      <tr>
        <td style="font-size:18px;font-weight:bold;color:#111;">%s account security</td>
      </tr>
      <tr><td style="padding-top:12px;font-size:15px;line-height:1.6;color:#111;">
        We detected activity that needs your review.
      </td></tr>
      <tr><td style="padding-top:12px;font-size:14px;line-height:1.6;color:#111;">
        <strong>Reason:</strong> %s<br/>
        <strong>Email:</strong> %s<br/>
        <strong>IP:</strong> %s<br/>
        <strong>Browser:</strong> %s<br/>
        <strong>Time (UTC):</strong> %s
      </td></tr>
      <tr><td style="padding-top:16px;">
        <a href="%s" style="display:inline-block;padding:12px 16px;background:#111827;color:#fff;text-decoration:none;border-radius:6px;font-size:14px;">Review devices</a>
      </td></tr>
      <tr><td style="padding-top:16px;font-size:13px;color:#6b7280;">
        If this wasn't you, reset your password and secure your account immediately.
      </td></tr>
    </table>
  </body>
</html>`, app, data.Reason, data.Email, data.IP, data.UserAgent, data.Timestamp, data.ManageURL)
}
