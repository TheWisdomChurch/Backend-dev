package service

import (
	"errors"
	"fmt"
	"html"
	"strings"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
)

func normalizeApprovalRole(role string) string {
	cleaned := strings.ToLower(strings.TrimSpace(role))
	cleaned = strings.ReplaceAll(cleaned, "-", "_")
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	return cleaned
}

func needsAdminApproval(user *models.User) bool {
	if user == nil {
		return false
	}
	return normalizeApprovalRole(user.Role) == "admin" && !user.AdminApproved
}

func adminDisplayName(user *models.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name != "" {
		return name
	}
	return strings.TrimSpace(user.Email)
}

func requestAdminApproval(approvalSvc ApprovalService, notifySvc AdminNotificationService, user *models.User) (*models.ApprovalRequest, error) {
	if !needsAdminApproval(user) {
		return nil, nil
	}
	if approvalSvc == nil {
		return nil, errors.New("admin approval service is not configured")
	}

	entityID := strings.TrimSpace(user.ID)
	if entityID == "" {
		return nil, errors.New("admin user id is required for approval request")
	}

	label := adminDisplayName(user)
	if label == "" {
		label = "Admin"
	}

	requestedByID := entityID
	requestedByName := adminDisplayName(user)
	requestedByEmail := strings.TrimSpace(user.Email)

	req, err := approvalSvc.CreateRequest(CreateApprovalRequest{
		Type:          models.ApprovalTypeAdminUser,
		EntityID:      &entityID,
		EntityLabel:   &label,
		RequestedByID: &requestedByID,
		RequestedByName: func() *string {
			if requestedByName == "" {
				return nil
			}
			return &requestedByName
		}(),
		RequestedByEmail: func() *string {
			if requestedByEmail == "" {
				return nil
			}
			return &requestedByEmail
		}(),
	})
	if err != nil {
		return nil, err
	}

	if notifySvc == nil {
		return req, nil
	}

	details := requestedByName
	if details == "" {
		details = requestedByEmail
	}
	if requestedByEmail != "" && requestedByName != "" {
		details = fmt.Sprintf("%s (%s)", requestedByName, requestedByEmail)
	}
	if strings.TrimSpace(details) == "" {
		details = "a new admin account"
	}

	title := "New admin approval request"
	message := fmt.Sprintf("A new admin account is awaiting super-admin approval for %s. Ticket %s.", details, req.TicketCode)
	entityType := "admin_user"

	_ = notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "admin_request",
		Title:      title,
		Message:    message,
		TicketCode: &req.TicketCode,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"super_admin"},
	})

	return req, nil
}

// requestAdminApprovalTx creates the approval request record only (no side-effects).
// Intended for use inside a database transaction. Call notifyAdminNewRegistration
// outside the transaction for the notification side-effect.
func requestAdminApprovalTx(approvalSvc ApprovalService, user *models.User) error {
	if !needsAdminApproval(user) {
		return nil
	}
	if approvalSvc == nil {
		return errors.New("admin approval service is not configured")
	}
	entityID := strings.TrimSpace(user.ID)
	if entityID == "" {
		return errors.New("admin user id is required for approval request")
	}
	label := adminDisplayName(user)
	if label == "" {
		label = "Admin"
	}
	requestedByID := entityID
	requestedByName := adminDisplayName(user)
	requestedByEmail := strings.TrimSpace(user.Email)
	var namePtr, emailPtr *string
	if requestedByName != "" {
		namePtr = &requestedByName
	}
	if requestedByEmail != "" {
		emailPtr = &requestedByEmail
	}
	_, err := approvalSvc.CreateRequest(CreateApprovalRequest{
		Type:             models.ApprovalTypeAdminUser,
		EntityID:         &entityID,
		EntityLabel:      &label,
		RequestedByID:    &requestedByID,
		RequestedByName:  namePtr,
		RequestedByEmail: emailPtr,
	})
	return err
}

// notifyAdminNewRegistration sends the admin-notification for a new registration.
// Call this outside any DB transaction.
func notifyAdminNewRegistration(notifySvc AdminNotificationService, user *models.User) {
	if notifySvc == nil || user == nil {
		return
	}
	entityID := strings.TrimSpace(user.ID)
	entityType := "admin_user"
	name := adminDisplayName(user)
	details := name
	if name == "" {
		details = "a new admin account"
	}
	title := "New admin approval request"
	message := fmt.Sprintf("A new admin account is awaiting super-admin approval for %s.", details)
	_ = notifySvc.NotifyRoles(AdminNotificationInput{
		Type:       "admin_request",
		Title:      title,
		Message:    message,
		EntityType: &entityType,
		EntityID:   &entityID,
		Roles:      []string{"super_admin"},
	})
}

func sendAdminApprovedEmail(sender EmailSender, branding email.Branding, user *models.User) {
	if sender == nil || user == nil || strings.TrimSpace(user.Email) == "" {
		return
	}
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	body := email.RenderAdminApprovedEmail(email.AdminApprovedTemplateData{
		Branding:      branding,
		RecipientName: name,
	})
	appName := strings.TrimSpace(branding.AppName)
	if appName == "" {
		appName = "The Wisdom Church"
	}
	subject := fmt.Sprintf("%s admin account approved", appName)
	_ = sender.SendHTML(user.Email, subject, body)
}

func sendAdminRejectedEmail(sender EmailSender, branding email.Branding, user *models.User, reason string) {
	if sender == nil || user == nil || strings.TrimSpace(user.Email) == "" {
		return
	}

	appName := strings.TrimSpace(branding.AppName)
	if appName == "" {
		appName = "The Wisdom Church"
	}

	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name == "" {
		name = "there"
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Your request did not meet the current access requirements."
	}

	body := fmt.Sprintf(`<!doctype html>
<html>
  <body style="margin:0;padding:0;background:#f8fafc;font-family:Arial,sans-serif;color:#111827;">
    <table width="100%%" cellpadding="0" cellspacing="0" style="padding:28px 14px;">
      <tr>
        <td align="center">
          <table width="100%%" cellpadding="0" cellspacing="0" style="max-width:640px;background:#ffffff;border:1px solid #e5e7eb;border-radius:16px;overflow:hidden;">
            <tr>
              <td style="padding:28px;">
                <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#b42318;">Access request update</div>
                <h1 style="margin:10px 0 12px;font-size:24px;line-height:1.25;color:#111827;">Your admin account request was not approved</h1>
                <p style="margin:0 0 14px;font-size:15px;line-height:1.7;color:#374151;">Hello %s,</p>
                <p style="margin:0 0 14px;font-size:15px;line-height:1.7;color:#374151;">Your request to create an admin account for %s was reviewed and rejected by the super admin.</p>
                <div style="margin:18px 0;padding:14px 16px;border-radius:12px;background:#fef3f2;border:1px solid #fecdca;color:#7a271a;font-size:14px;line-height:1.6;">%s</div>
                <p style="margin:0;font-size:13px;line-height:1.7;color:#6b7280;">If you believe this is a mistake, contact the church administration team.</p>
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`, html.EscapeString(name), html.EscapeString(appName), html.EscapeString(reason))
	body = email.EnsureResponsiveDocument(branding, body)

	_ = sender.SendHTML(user.Email, fmt.Sprintf("%s admin account request update", appName), body)
}
