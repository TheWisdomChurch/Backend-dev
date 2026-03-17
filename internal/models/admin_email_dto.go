package models

type AdminEmailRecipientInput struct {
	Name  *string `json:"name,omitempty"`
	Email string  `json:"email"`
}

type AdminEmailAudienceFormSummary struct {
	FormID           string `json:"formId"`
	FormTitle        string `json:"formTitle"`
	TotalSubmissions int    `json:"totalSubmissions"`
	ValidRecipients  int    `json:"validRecipients"`
	UniqueRecipients int    `json:"uniqueRecipients"`
}

type SendAdminComposeEmailRequest struct {
	Subject          *string                     `json:"subject,omitempty"`
	HTMLBody         *string                     `json:"htmlBody,omitempty"`
	TextBody         *string                     `json:"textBody,omitempty"`
	TemplateID       *string                     `json:"templateId,omitempty"`
	TemplateKey      *string                     `json:"templateKey,omitempty"`
	ManualRecipients *[]AdminEmailRecipientInput `json:"manualRecipients,omitempty"`
	FormIDs          *[]string                   `json:"formIds,omitempty"`
}

type SendAdminComposeEmailResponse struct {
	DeliveryID       *string                         `json:"deliveryId,omitempty"`
	Subject          string                          `json:"subject"`
	TemplateSource   string                          `json:"templateSource"`
	AudienceSource   string                          `json:"audienceSource"`
	ManualRecipients int                             `json:"manualRecipients"`
	FormRecipients   int                             `json:"formRecipients"`
	SourceForms      []AdminEmailAudienceFormSummary `json:"sourceForms,omitempty"`
	TotalRecipients  int                             `json:"totalRecipients"`
	Targeted         int                             `json:"targeted"`
	Sent             int                             `json:"sent"`
	Skipped          int                             `json:"skipped"`
	Failed           int                             `json:"failed"`
	FailedRecipients []string                        `json:"failedRecipients,omitempty"`
	StartedAt        string                          `json:"startedAt"`
	CompletedAt      string                          `json:"completedAt"`
	SentAt           string                          `json:"sentAt"`
}

type AdminEmailDeliveryHistoryItem struct {
	ID               string                          `json:"id"`
	Subject          string                          `json:"subject"`
	TemplateSource   string                          `json:"templateSource"`
	TemplateID       *string                         `json:"templateId,omitempty"`
	TemplateKey      *string                         `json:"templateKey,omitempty"`
	AudienceSource   string                          `json:"audienceSource"`
	ManualRecipients int                             `json:"manualRecipients"`
	FormRecipients   int                             `json:"formRecipients"`
	SourceForms      []AdminEmailAudienceFormSummary `json:"sourceForms,omitempty"`
	Status           string                          `json:"status"`
	TotalRecipients  int                             `json:"totalRecipients"`
	Targeted         int                             `json:"targeted"`
	Sent             int                             `json:"sent"`
	Skipped          int                             `json:"skipped"`
	Failed           int                             `json:"failed"`
	FailedRecipients []string                        `json:"failedRecipients,omitempty"`
	StartedAt        string                          `json:"startedAt"`
	CompletedAt      *string                         `json:"completedAt,omitempty"`
	CreatedByUserID  *string                         `json:"createdByUserId,omitempty"`
	CreatedByEmail   *string                         `json:"createdByEmail,omitempty"`
	CreatedByRole    *string                         `json:"createdByRole,omitempty"`
	CreatedAt        string                          `json:"createdAt"`
	UpdatedAt        string                          `json:"updatedAt"`
}
