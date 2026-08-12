package models

import "time"

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

// AdminEmailAttachmentInput references a file already uploaded through the
// existing /admin/uploads pipeline (images, documents, etc.) — the compose
// request carries only the public URL and display filename; the backend
// fetches the bytes server-side at send time rather than accepting raw
// payloads in the JSON body.
type AdminEmailAttachmentInput struct {
	URL      string `json:"url"`
	Filename string `json:"filename,omitempty"`
}

type SendAdminComposeEmailRequest struct {
	Subject          *string                      `json:"subject,omitempty"`
	HTMLBody         *string                      `json:"htmlBody,omitempty"`
	TextBody         *string                      `json:"textBody,omitempty"`
	TemplateID       *string                      `json:"templateId,omitempty"`
	TemplateKey      *string                      `json:"templateKey,omitempty"`
	ManualRecipients *[]AdminEmailRecipientInput  `json:"manualRecipients,omitempty"`
	FormIDs          *[]string                    `json:"formIds,omitempty"`
	AudienceTypes    *[]string                    `json:"audienceTypes,omitempty"`
	Attachments      *[]AdminEmailAttachmentInput `json:"attachments,omitempty"`
}

type UpsertAdminEmailScheduleRequest struct {
	Name          string                       `json:"name" binding:"required"`
	Description   string                       `json:"description,omitempty"`
	Status        AdminEmailScheduleStatus     `json:"status,omitempty"`
	Recurrence    AdminEmailRecurrence         `json:"recurrence" binding:"required"`
	Timezone      string                       `json:"timezone" binding:"required"`
	SendTime      string                       `json:"sendTime" binding:"required"`
	StartDate     string                       `json:"startDate" binding:"required"`
	EndDate       *string                      `json:"endDate,omitempty"`
	Weekdays      []int                        `json:"weekdays,omitempty"`
	MonthDays     []int                        `json:"monthDays,omitempty"`
	StartAt       time.Time                    `json:"startAt,omitempty"`
	EndAt         *time.Time                   `json:"endAt,omitempty"`
	AudienceLabel string                       `json:"audienceLabel,omitempty"`
	Compose       SendAdminComposeEmailRequest `json:"compose" binding:"required"`
}

type AdminEmailScheduleDetail struct {
	AdminEmailSchedule
	Compose SendAdminComposeEmailRequest `json:"compose"`
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
	DuplicateRecipients int                          `json:"duplicateRecipients"`
	UnsubscribedRecipients int                       `json:"unsubscribedRecipients"`
	InvalidRecipients int                            `json:"invalidRecipients"`
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

type AdminEmailMarketingFormItem struct {
	FormID           string  `json:"formId"`
	FormTitle        string  `json:"formTitle"`
	Status           string  `json:"status"`
	IsPublished      bool    `json:"isPublished"`
	PublicURL        *string `json:"publicUrl,omitempty"`
	PublishedAt      *string `json:"publishedAt,omitempty"`
	UpdatedAt        string  `json:"updatedAt"`
	LastSubmissionAt *string `json:"lastSubmissionAt,omitempty"`
	TotalSubmissions int     `json:"totalSubmissions"`
	ValidRecipients  int     `json:"validRecipients"`
	UniqueRecipients int     `json:"uniqueRecipients"`
}

type AdminEmailMarketingSummary struct {
	TotalForms          int                             `json:"totalForms"`
	PublishedForms      int                             `json:"publishedForms"`
	DraftForms          int                             `json:"draftForms"`
	TotalSubmissions    int                             `json:"totalSubmissions"`
	ReachableRecipients int                             `json:"reachableRecipients"`
	TotalCampaigns      int64                           `json:"totalCampaigns"`
	TopForms            []AdminEmailMarketingFormItem   `json:"topForms,omitempty"`
	RecentCampaigns     []AdminEmailDeliveryHistoryItem `json:"recentCampaigns,omitempty"`
}

type AdminEmailAudienceRecipientSource struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	FormID    string `json:"formId,omitempty"`
	FormTitle string `json:"formTitle,omitempty"`
}

type AdminEmailAudiencePreviewRecipient struct {
	Email       string                              `json:"email"`
	Name        string                              `json:"name,omitempty"`
	SourceForms []AdminEmailAudienceRecipientSource `json:"sourceForms,omitempty"`
	Sources     []AdminEmailAudienceRecipientSource `json:"sources,omitempty"`
	Duplicate  bool                                 `json:"duplicate"`
}

type AdminEmailAudiencePreview struct {
	Forms            []AdminEmailMarketingFormItem        `json:"forms"`
	TotalForms       int                                  `json:"totalForms"`
	TotalSubmissions int                                  `json:"totalSubmissions"`
	ValidRecipients  int                                  `json:"validRecipients"`
	UniqueRecipients int                                  `json:"uniqueRecipients"`
	Skipped          int                                  `json:"skipped"`
	DuplicateRecipients int                               `json:"duplicateRecipients"`
	InvalidRecipients int                                 `json:"invalidRecipients"`
	PreviewCount     int                                  `json:"previewCount"`
	Recipients       []AdminEmailAudiencePreviewRecipient `json:"recipients"`
}
