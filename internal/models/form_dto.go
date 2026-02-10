// internal/models/form_dto.go
package models

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

type FormSettingsDTO struct {
	Capacity       *int    `json:"capacity,omitempty"`
	ClosesAt       *string `json:"closesAt,omitempty" binding:"omitempty,rfc3339"`  // ISO string
	ExpiresAt      *string `json:"expiresAt,omitempty" binding:"omitempty,rfc3339"` // ISO string
	SuccessMessage *string `json:"successMessage,omitempty"`                        // optional

	// Response email (auto-reply after submission)
	ResponseEmailEnabled     *bool   `json:"responseEmailEnabled,omitempty"`
	ResponseEmailTemplateID  *string `json:"responseEmailTemplateId,omitempty"`
	ResponseEmailTemplateKey *string `json:"responseEmailTemplateKey,omitempty"`
	ResponseEmailSubject     *string `json:"responseEmailSubject,omitempty"`

	Design *FormDesignSettingsDTO `json:"design,omitempty"`

	// Extended builder settings (kept at root for backward compat with frontend)
	IntroTitle            *string   `json:"introTitle,omitempty"`
	IntroSubtitle         *string   `json:"introSubtitle,omitempty"`
	IntroBullets          *[]string `json:"introBullets,omitempty"`
	IntroBulletSubtexts   *[]string `json:"introBulletSubtexts,omitempty"`
	LayoutMode            *string   `json:"layoutMode,omitempty"`
	DateFormat            *string   `json:"dateFormat,omitempty"`
	FooterText            *string   `json:"footerText,omitempty"`
	FooterBg              *string   `json:"footerBg,omitempty"`
	FooterTextColor       *string   `json:"footerTextColor,omitempty"`
	SubmitButtonText      *string   `json:"submitButtonText,omitempty"`
	SubmitButtonBg        *string   `json:"submitButtonBg,omitempty"`
	SubmitButtonTextColor *string   `json:"submitButtonTextColor,omitempty"`
	SubmitButtonIcon      *string   `json:"submitButtonIcon,omitempty"`
	FormHeaderNote        *string   `json:"formHeaderNote,omitempty"`
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

type PublicFormPayload struct {
	Form  *Form  `json:"form"`
	Event *Event `json:"event,omitempty"`
}
