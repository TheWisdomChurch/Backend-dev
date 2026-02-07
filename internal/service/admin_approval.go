package service

import (
	"fmt"
	"strings"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
)

func needsAdminApproval(user *models.User) bool {
	if user == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(user.Role), "admin") && !user.AdminApproved
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

func requestAdminApproval(approvalSvc ApprovalService, notifySvc AdminNotificationService, user *models.User) {
	if approvalSvc == nil || !needsAdminApproval(user) {
		return
	}

	entityID := strings.TrimSpace(user.ID)
	if entityID == "" {
		return
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
		return
	}

	if notifySvc == nil {
		return
	}

	details := requestedByName
	if details == "" {
		details = requestedByEmail
	}
	if requestedByEmail != "" && requestedByName != "" {
		details = fmt.Sprintf("%s (%s)", requestedByName, requestedByEmail)
	}

	title := "New admin approval request"
	message := fmt.Sprintf("A new admin account is awaiting approval for %s. Ticket %s.", details, req.TicketCode)
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
		appName = "Wisdom House"
	}
	subject := fmt.Sprintf("%s admin account approved", appName)
	_ = sender.SendHTML(user.Email, subject, body)
}
