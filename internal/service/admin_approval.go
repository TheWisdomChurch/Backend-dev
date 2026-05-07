package service

import (
	"fmt"
	"strings"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
)

func normalizeAdminRoleForApproval(role string) string {
	cleaned := strings.ToLower(strings.TrimSpace(role))
	cleaned = strings.ReplaceAll(cleaned, "-", "_")
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	if cleaned == "superadmin" {
		return "super_admin"
	}
	return cleaned
}

func needsAdminApproval(user *models.User) bool {
	if user == nil {
		return false
	}

	role := normalizeAdminRoleForApproval(user.Role)
	return (role == "admin" || role == "super_admin") && !user.AdminApproved
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

	role := normalizeAdminRoleForApproval(user.Role)
	if role == "" {
		role = "admin"
	}

	label := adminDisplayName(user)
	if label == "" {
		label = "Admin"
	}
	label = fmt.Sprintf("%s access request: %s", role, label)

	requestedByID := entityID
	requestedByName := adminDisplayName(user)
	requestedByEmail := strings.TrimSpace(user.Email)

	req, err := approvalSvc.CreateRequest(CreateApprovalRequest{
		Type:             models.ApprovalTypeAdminUser,
		EntityID:         &entityID,
		EntityLabel:      &label,
		RequestedByID:    &requestedByID,
		RequestedByName:  stringPtrIfNotBlank(requestedByName),
		RequestedByEmail: stringPtrIfNotBlank(requestedByEmail),
	})
	if err != nil || req == nil {
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
	if details == "" {
		details = "a new admin user"
	}

	title := "New admin approval request"
	message := fmt.Sprintf("A new %s account is awaiting super-admin approval for %s. Ticket %s.", role, details, req.TicketCode)
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

func stringPtrIfNotBlank(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
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
