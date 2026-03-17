// internal/models/form_dto.go
package models

import "time"

type FormFieldOptionDTO struct {
	Label string `json:"label" binding:"required"`
	Value string `json:"value" binding:"required"`
}

type FormFieldDTO struct {
	ID         string               `json:"id,omitempty"`
	Key        string               `json:"key,omitempty"`
	Label      string               `json:"label" binding:"required"`
	Type       string               `json:"type" binding:"required"`
	Required   bool                 `json:"required"`
	Options    []FormFieldOptionDTO `json:"options,omitempty" binding:"omitempty,dive"`
	Validation *FormFieldValidation `json:"validation,omitempty"`
	Visibility *FormFieldVisibility `json:"visibility,omitempty"`
	Order      int                  `json:"order"`
}

type FormDesignSettingsDTO struct {
	HeroTitle       *string `json:"heroTitle,omitempty"`
	HeroSubtitle    *string `json:"heroSubtitle,omitempty"`
	CoverImageURL   *string `json:"coverImageUrl,omitempty"`
	PrimaryColor    *string `json:"primaryColor,omitempty"`
	BackgroundColor *string `json:"backgroundColor,omitempty"`
	AccentColor     *string `json:"accentColor,omitempty"`
	Layout          *string `json:"layout,omitempty"` // e.g. split, stacked
	CTAButtonLabel  *string `json:"ctaButtonLabel,omitempty"`
	PrivacyCopy     *string `json:"privacyCopy,omitempty"`
	FooterNote      *string `json:"footerNote,omitempty"`

	// Extended footer controls for the new builder
	FooterText      *string `json:"footerText,omitempty"`
	FooterBg        *string `json:"footerBg,omitempty"`
	FooterTextColor *string `json:"footerTextColor,omitempty"`

	// Button styling
	SubmitButtonText      *string `json:"submitButtonText,omitempty"`
	SubmitButtonBg        *string `json:"submitButtonBg,omitempty"`
	SubmitButtonTextColor *string `json:"submitButtonTextColor,omitempty"`
	SubmitButtonIcon      *string `json:"submitButtonIcon,omitempty"` // check|send|calendar|cursor|none

	// Hero/intro copy used in builder previews
	IntroTitle         *string   `json:"introTitle,omitempty"`
	IntroSubtitle      *string   `json:"introSubtitle,omitempty"`
	IntroBullets       *[]string `json:"introBullets,omitempty"`
	IntroBulletSubtext *[]string `json:"introBulletSubtexts,omitempty"`
	LayoutMode         *string   `json:"layoutMode,omitempty"` // split|stack
	DateFormat         *string   `json:"dateFormat,omitempty"` // yyyy-mm-dd etc
	FormHeaderNote     *string   `json:"formHeaderNote,omitempty"`
}

type FormContentSectionItemDTO struct {
	Title    string  `json:"title" binding:"required"`
	Body     *string `json:"body,omitempty"`
	Eyebrow  *string `json:"eyebrow,omitempty"`
	Icon     *string `json:"icon,omitempty"`
	LinkText *string `json:"linkText,omitempty"`
	LinkURL  *string `json:"linkUrl,omitempty"`
}

type FormContentSectionDTO struct {
	ID       *string                     `json:"id,omitempty"`
	Title    string                      `json:"title" binding:"required"`
	Subtitle *string                     `json:"subtitle,omitempty"`
	Layout   *string                     `json:"layout,omitempty"` // grid|stack|timeline|split
	Items    []FormContentSectionItemDTO `json:"items,omitempty"`
}

type FormSettingsDTO struct {
	Capacity       *int    `json:"capacity,omitempty"`
	ClosesAt       *string `json:"closesAt,omitempty" binding:"omitempty,rfc3339"`  // ISO string
	ExpiresAt      *string `json:"expiresAt,omitempty" binding:"omitempty,rfc3339"` // ISO string
	SuccessMessage *string `json:"successMessage,omitempty"`                        // optional

	// Form type controls public header label (registration, membership, workforce, etc).
	FormType *string `json:"formType,omitempty"`

	// Response email (auto-reply after submission)
	ResponseEmailEnabled     *bool   `json:"responseEmailEnabled,omitempty"`
	ResponseEmailTemplateID  *string `json:"responseEmailTemplateId,omitempty"`
	ResponseEmailTemplateKey *string `json:"responseEmailTemplateKey,omitempty"`
	ResponseEmailTemplateURL *string `json:"responseEmailTemplateUrl,omitempty"`
	ResponseEmailSubject     *string `json:"responseEmailSubject,omitempty"`

	// Campaign email (bulk outreach to form registrants)
	CampaignEmailEnabled     *bool   `json:"campaignEmailEnabled,omitempty"`
	CampaignEmailTemplateID  *string `json:"campaignEmailTemplateId,omitempty"`
	CampaignEmailTemplateKey *string `json:"campaignEmailTemplateKey,omitempty"`
	CampaignEmailTemplateURL *string `json:"campaignEmailTemplateUrl,omitempty"`
	CampaignEmailSubject     *string `json:"campaignEmailSubject,omitempty"`

	// Submission routing (optional): send submissions into workforce/members tables.
	SubmissionTarget     *string `json:"submissionTarget,omitempty"`     // workforce|workforce_new|workforce_serving|member
	SubmissionDepartment *string `json:"submissionDepartment,omitempty"` // default department for workforce

	Design *FormDesignSettingsDTO `json:"design,omitempty"`

	// Extended builder settings (kept at root for backward compat with frontend)
	IntroTitle            *string                  `json:"introTitle,omitempty"`
	IntroSubtitle         *string                  `json:"introSubtitle,omitempty"`
	IntroBullets          *[]string                `json:"introBullets,omitempty"`
	IntroBulletSubtexts   *[]string                `json:"introBulletSubtexts,omitempty"`
	LayoutMode            *string                  `json:"layoutMode,omitempty"`
	DateFormat            *string                  `json:"dateFormat,omitempty"`
	FooterText            *string                  `json:"footerText,omitempty"`
	FooterBg              *string                  `json:"footerBg,omitempty"`
	FooterTextColor       *string                  `json:"footerTextColor,omitempty"`
	Sections              *[]FormContentSectionDTO `json:"sections,omitempty"`
	SubmitButtonText      *string                  `json:"submitButtonText,omitempty"`
	SubmitButtonBg        *string                  `json:"submitButtonBg,omitempty"`
	SubmitButtonTextColor *string                  `json:"submitButtonTextColor,omitempty"`
	SubmitButtonIcon      *string                  `json:"submitButtonIcon,omitempty"`
	FormHeaderNote        *string                  `json:"formHeaderNote,omitempty"`
}

type CreateFormRequest struct {
	Title       string           `json:"title" binding:"required"`
	Description *string          `json:"description,omitempty"`
	EventID     *string          `json:"eventId,omitempty"`
	Slug        *string          `json:"slug,omitempty"`
	Settings    *FormSettingsDTO `json:"settings,omitempty"`
	Fields      []FormFieldDTO   `json:"fields" binding:"omitempty,dive"`
}

type UpdateFormRequest struct {
	Title       *string          `json:"title,omitempty"`
	Description *string          `json:"description,omitempty"`
	EventID     *string          `json:"eventId,omitempty"`
	Slug        *string          `json:"slug,omitempty"`
	Settings    *FormSettingsDTO `json:"settings,omitempty"`
	Fields      *[]FormFieldDTO  `json:"fields,omitempty"` // if provided, replaces fields
}

type SubmitFormRequest struct {
	Values map[string]any `json:"values" binding:"required"`
}

type FormCampaignEmailCTA struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type FormCampaignEmailHighlight struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type FormCampaignEmailResource struct {
	Label       string  `json:"label"`
	URL         string  `json:"url"`
	Description *string `json:"description,omitempty"`
	Kind        *string `json:"kind,omitempty"`
}

type SendFormCampaignEmailRequest struct {
	Subject              *string                       `json:"subject,omitempty"`
	Title                *string                       `json:"title,omitempty"`
	PreviewText          *string                       `json:"previewText,omitempty"`
	HeroEyebrow          *string                       `json:"heroEyebrow,omitempty"`
	HeroTitle            *string                       `json:"heroTitle,omitempty"`
	HeroSubtitle         *string                       `json:"heroSubtitle,omitempty"`
	HTMLBody             *string                       `json:"htmlBody,omitempty"`
	TextBody             *string                       `json:"textBody,omitempty"`
	IntroHTML            *string                       `json:"introHtml,omitempty"`
	BodyHTML             *string                       `json:"bodyHtml,omitempty"`
	ClosingHTML          *string                       `json:"closingHtml,omitempty"`
	HeroImageURL         *string                       `json:"heroImageUrl,omitempty"`
	FlyerImageURLs       *[]string                     `json:"flyerImageUrls,omitempty"`
	PrimaryCTA           *FormCampaignEmailCTA         `json:"primaryCta,omitempty"`
	SecondaryCTA         *FormCampaignEmailCTA         `json:"secondaryCta,omitempty"`
	Highlights           *[]FormCampaignEmailHighlight `json:"highlights,omitempty"`
	ResourceLinks        *[]FormCampaignEmailResource  `json:"resourceLinks,omitempty"`
	FooterNote           *string                       `json:"footerNote,omitempty"`
	TargetSubmissionIDs  *[]string                     `json:"targetSubmissionIds,omitempty"`
	IncludeCalendarLinks *bool                         `json:"includeCalendarLinks,omitempty"`
	TemplateID           *string                       `json:"templateId,omitempty"`
	TemplateKey          *string                       `json:"templateKey,omitempty"`
}

type SendFormCampaignEmailResponse struct {
	DeliveryID       *string  `json:"deliveryId,omitempty"`
	FormID           string   `json:"formId"`
	FormTitle        string   `json:"formTitle"`
	EventTitle       *string  `json:"eventTitle,omitempty"`
	Subject          string   `json:"subject"`
	TemplateSource   string   `json:"templateSource"`
	TotalRecipients  int      `json:"totalRecipients"`
	Targeted         int      `json:"targeted"`
	Sent             int      `json:"sent"`
	Skipped          int      `json:"skipped"`
	Failed           int      `json:"failed"`
	FailedRecipients []string `json:"failedRecipients,omitempty"`
	StartedAt        string   `json:"startedAt"`
	CompletedAt      string   `json:"completedAt"`
	SentAt           string   `json:"sentAt"`
}

type FormCampaignDeliveryHistoryItem struct {
	ID               string   `json:"id"`
	FormID           string   `json:"formId"`
	FormTitle        string   `json:"formTitle"`
	EventID          *string  `json:"eventId,omitempty"`
	EventTitle       *string  `json:"eventTitle,omitempty"`
	Subject          string   `json:"subject"`
	TemplateSource   string   `json:"templateSource"`
	TemplateID       *string  `json:"templateId,omitempty"`
	TemplateKey      *string  `json:"templateKey,omitempty"`
	Status           string   `json:"status"`
	TotalRecipients  int      `json:"totalRecipients"`
	Targeted         int      `json:"targeted"`
	Sent             int      `json:"sent"`
	Skipped          int      `json:"skipped"`
	Failed           int      `json:"failed"`
	FailedRecipients []string `json:"failedRecipients,omitempty"`
	StartedAt        string   `json:"startedAt"`
	CompletedAt      *string  `json:"completedAt,omitempty"`
	CreatedByUserID  *string  `json:"createdByUserId,omitempty"`
	CreatedByEmail   *string  `json:"createdByEmail,omitempty"`
	CreatedByRole    *string  `json:"createdByRole,omitempty"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

type PublicFormPayload struct {
	Form  *Form  `json:"form"`
	Event *Event `json:"event,omitempty"`
}

type FormReportLinkPayload struct {
	FormID        string `json:"formId"`
	FormTitle     string `json:"formTitle"`
	Slug          string `json:"slug"`
	ReportURL     string `json:"reportUrl"`
	ReportDataURL string `json:"reportDataUrl"`
	ExportPDFURL  string `json:"exportPdfUrl"`
}

type FormReportSummary struct {
	TotalSubmissions   int64      `json:"totalSubmissions"`
	LatestSubmissionAt *time.Time `json:"latestSubmissionAt,omitempty"`
}

type FormReportFieldValue struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type FormReportSubmission struct {
	ID               string                 `json:"id"`
	Name             *string                `json:"name,omitempty"`
	Email            *string                `json:"email,omitempty"`
	ContactNumber    *string                `json:"contactNumber,omitempty"`
	ContactAddress   *string                `json:"contactAddress,omitempty"`
	RegistrationCode *string                `json:"registrationCode,omitempty"`
	Values           map[string]any         `json:"values,omitempty"`
	Fields           []FormReportFieldValue `json:"fields,omitempty"`
	CreatedAt        time.Time              `json:"createdAt"`
}

type PublicFormReportPayload struct {
	BrandName           string                 `json:"brandName"`
	FormID              string                 `json:"formId"`
	FormTitle           string                 `json:"formTitle"`
	FormDescription     *string                `json:"formDescription,omitempty"`
	Slug                string                 `json:"slug"`
	Summary             FormReportSummary      `json:"summary"`
	LatestRegistrations []FormReportSubmission `json:"latestRegistrations"`
	Submissions         []FormReportSubmission `json:"submissions"`
	Page                int                    `json:"page"`
	Limit               int                    `json:"limit"`
	Total               int64                  `json:"total"`
	TotalPages          int                    `json:"totalPages"`
	ReportURL           string                 `json:"reportUrl"`
	ReportDataURL       string                 `json:"reportDataUrl"`
	ExportPDFURL        string                 `json:"exportPdfUrl"`
	GeneratedAt         time.Time              `json:"generatedAt"`
}
