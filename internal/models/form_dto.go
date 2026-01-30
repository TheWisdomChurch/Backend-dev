// internal/models/form_dto.go
package models

type FormFieldOptionDTO struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type FormFieldDTO struct {
	ID       string               `json:"id,omitempty"`
	Key      string               `json:"key"`
	Label    string               `json:"label"`
	Type     string               `json:"type"`
	Required bool                 `json:"required"`
	Options  []FormFieldOptionDTO `json:"options,omitempty"`
	Order    int                  `json:"order"`
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
}

type FormSettingsDTO struct {
	Capacity       *int                   `json:"capacity,omitempty"`
	ClosesAt       *string                `json:"closesAt,omitempty"`       // ISO string
	ExpiresAt      *string                `json:"expiresAt,omitempty"`      // ISO string
	SuccessMessage *string                `json:"successMessage,omitempty"` // optional
	Design         *FormDesignSettingsDTO `json:"design,omitempty"`
}

type CreateFormRequest struct {
	Title       string           `json:"title"`
	Description *string          `json:"description,omitempty"`
	EventID     *string          `json:"eventId,omitempty"`
	Settings    *FormSettingsDTO `json:"settings,omitempty"`
	Fields      []FormFieldDTO   `json:"fields"`
}

type UpdateFormRequest struct {
	Title       *string          `json:"title,omitempty"`
	Description *string          `json:"description,omitempty"`
	EventID     *string          `json:"eventId,omitempty"`
	Settings    *FormSettingsDTO `json:"settings,omitempty"`
	Fields      *[]FormFieldDTO  `json:"fields,omitempty"` // if provided, replaces fields
}

type SubmitFormRequest struct {
	Values map[string]any `json:"values"`
}

type PublicFormPayload struct {
	Form  *Form  `json:"form"`
	Event *Event `json:"event,omitempty"`
}
