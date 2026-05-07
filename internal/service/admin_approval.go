package service

import (
	"errors"
	"fmt"
	"strings"

	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/models"
)

func needsAdminApproval(user *models.User) bool {
	if user == nil {
		return false
	}
	role := strings.ToLower(strings.TrimSpace(user.Role))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	return role == "admin" && !user.AdminApproved
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
