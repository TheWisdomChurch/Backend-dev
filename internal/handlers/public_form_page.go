package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
)

type publicFormPageFieldView struct {
	Key            string
	Label          string
	Type           string
	Required       bool
	Options        []models.FormFieldOptionDTO
	VisibilityJSON string
}

type publicFormPageData struct {
	Form           *models.Form
	Settings       *models.FormSettingsDTO
	Fields         []publicFormPageFieldView
	SubmitURL      string
	SubmitLabel    string
	SuccessMessage string
	SubmissionType string
	IsLeadership   bool
	HomeURL        string
	CoverImageUrl  string
	DPGeneratorURL string
}

func (h *FormHandler) ViewPublicFormPage(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		h.renderPublicFormPageError(c, http.StatusNotFound, "Form not found", "This form link is invalid or unavailable.")
		return
	}

	payload, err := h.svc.GetPublic(slug)
	if err != nil {
		switch err {
		case gorm.ErrRecordNotFound:
			h.renderPublicFormPageError(c, http.StatusNotFound, "Form not found", "This form link is invalid or unavailable.")
		case service.ErrFormExpired, service.ErrFormClosed:
			h.renderPublicFormPageError(c, http.StatusGone, "Registration closed", err.Error())
		default:
			h.renderPublicFormPageError(c, http.StatusBadRequest, "Unable to load form", err.Error())
		}
		return
	}

	settings := decodeFormSettings(payload.Form.Settings)
	submissionType := resolveFormSubmissionType(payload.Form, settings)

	var coverImageUrl string
	if settings != nil {
		if settings.Design != nil && settings.Design.CoverImageURL != nil && strings.TrimSpace(*settings.Design.CoverImageURL) != "" {
			coverImageUrl = strings.TrimSpace(*settings.Design.CoverImageURL)
		} else if settings.AttendingBadge != nil && settings.AttendingBadge.CustomFlyerURL != nil && strings.TrimSpace(*settings.AttendingBadge.CustomFlyerURL) != "" {
			coverImageUrl = strings.TrimSpace(*settings.AttendingBadge.CustomFlyerURL)
		}
	}

	dpURL := "/forms/" + slug + "/dp"

	data := publicFormPageData{
		Form:           payload.Form,
		Settings:       settings,
		Fields:         buildPublicFormFieldViews(payload.Form.Fields),
		SubmitURL:      "/api/v1/forms/" + slug + "/submissions",
		SubmitLabel:    "Submit Registration",
		SuccessMessage: "Registration submitted successfully.",
		SubmissionType: submissionType,
		IsLeadership:   submissionType == "leadership",
		HomeURL:        "/",
		CoverImageUrl:  coverImageUrl,
		DPGeneratorURL: dpURL,
	}
	if settings != nil && settings.SuccessMessage != nil && strings.TrimSpace(*settings.SuccessMessage) != "" {
		data.SuccessMessage = strings.TrimSpace(*settings.SuccessMessage)
	}
	if settings != nil && settings.SubmitButtonText != nil && strings.TrimSpace(*settings.SubmitButtonText) != "" {
		data.SubmitLabel = strings.TrimSpace(*settings.SubmitButtonText)
	}

	tpl := template.Must(template.New("public-form-page").Funcs(template.FuncMap{
		"renderFieldInput": renderFieldInput,
		"hasText":          hasText,
		"text":             pointerText,
		"layoutClass":      sectionLayoutClass,
	}).Parse(publicFormPageHTML))

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	_ = tpl.Execute(c.Writer, data)
}

func (h *FormHandler) RedirectLegacyPublicFormPage(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		h.renderPublicFormPageError(c, http.StatusNotFound, "Form not found", "This form link is invalid or unavailable.")
		return
	}

	target := "/forms/" + url.PathEscape(slug)
	if rawQuery := strings.TrimSpace(c.Request.URL.RawQuery); rawQuery != "" {
		target += "?" + rawQuery
	}

	c.Redirect(http.StatusPermanentRedirect, target)
}

func decodeFormSettings(raw datatypes.JSON) *models.FormSettingsDTO {
	if len(raw) == 0 || string(raw) == "null" {
		return &models.FormSettingsDTO{}
	}

	var settings models.FormSettingsDTO
	if err := json.Unmarshal(raw, &settings); err != nil {
		return &models.FormSettingsDTO{}
	}
	return &settings
}

func buildPublicFormFieldViews(fields []models.FormField) []publicFormPageFieldView {
	views := make([]publicFormPageFieldView, 0, len(fields))
	for _, field := range fields {
		views = append(views, publicFormPageFieldView{
			Key:            strings.TrimSpace(field.Key),
			Label:          strings.TrimSpace(field.Label),
			Type:           strings.TrimSpace(string(field.Type)),
			Required:       field.Required,
			Options:        decodeFieldOptions(field.Options),
			VisibilityJSON: decodeFieldVisibilityJSON(field.Visibility),
		})
	}
	return views
}

func decodeFieldOptions(raw datatypes.JSON) []models.FormFieldOptionDTO {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var options []models.FormFieldOptionDTO
	_ = json.Unmarshal(raw, &options)
	return options
}

func decodeFieldVisibilityJSON(raw datatypes.JSON) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

func renderFieldInput(field publicFormPageFieldView) template.HTML {
	requiredAttr := ""
	requiredBadge := ""
	if field.Required {
		requiredAttr = " required"
		requiredBadge = " <span class=\"required\">*</span>"
	}

	label := template.HTMLEscapeString(field.Label)
	key := template.HTMLEscapeString(field.Key)
	inputType := template.HTMLEscapeString(field.Type)

	switch field.Type {
	case string(models.FieldTextarea):
		return template.HTML("<label class=\"field-label\" for=\"" + key + "\">" + label + requiredBadge + "</label><textarea class=\"field-control textarea\" id=\"" + key + "\" name=\"" + key + "\"" + requiredAttr + "></textarea>")
	case string(models.FieldSelect):
		var builder strings.Builder
		builder.WriteString("<label class=\"field-label\" for=\"" + key + "\">" + label + requiredBadge + "</label>")
		builder.WriteString("<select class=\"field-control\" id=\"" + key + "\" name=\"" + key + "\"" + requiredAttr + ">")
		builder.WriteString("<option value=\"\">Select an option</option>")
		for _, option := range field.Options {
			builder.WriteString("<option value=\"" + template.HTMLEscapeString(option.Value) + "\">" + template.HTMLEscapeString(option.Label) + "</option>")
		}
		builder.WriteString("</select>")
		return template.HTML(builder.String())
	case string(models.FieldRadio):
		var builder strings.Builder
		builder.WriteString("<span class=\"field-label\">" + label + requiredBadge + "</span><div class=\"choice-group\">")
		for _, option := range field.Options {
			builder.WriteString("<label class=\"choice\"><input type=\"radio\" name=\"" + key + "\" value=\"" + template.HTMLEscapeString(option.Value) + "\"" + requiredAttr + "><span>" + template.HTMLEscapeString(option.Label) + "</span></label>")
		}
		builder.WriteString("</div>")
		return template.HTML(builder.String())
	case string(models.FieldCheckbox):
		if len(field.Options) == 0 {
			return template.HTML("<label class=\"choice single\"><input type=\"checkbox\" id=\"" + key + "\" name=\"" + key + "\"" + requiredAttr + "><span>" + label + requiredBadge + "</span></label>")
		}
		var builder strings.Builder
		builder.WriteString("<span class=\"field-label\">" + label + requiredBadge + "</span><div class=\"choice-group\">")
		for _, option := range field.Options {
			builder.WriteString("<label class=\"choice\"><input type=\"checkbox\" name=\"" + key + "\" value=\"" + template.HTMLEscapeString(option.Value) + "\"><span>" + template.HTMLEscapeString(option.Label) + "</span></label>")
		}
		builder.WriteString("</div>")
		return template.HTML(builder.String())
	default:
		htmlType := inputType
		if htmlType == "" {
			htmlType = "text"
		}
		return template.HTML("<label class=\"field-label\" for=\"" + key + "\">" + label + requiredBadge + "</label><input class=\"field-control\" type=\"" + htmlType + "\" id=\"" + key + "\" name=\"" + key + "\"" + requiredAttr + ">")
	}
}

func hasText(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func pointerText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func sectionLayoutClass(value *string) string {
	if value == nil {
		return "section-grid"
	}
	switch strings.ToLower(strings.TrimSpace(*value)) {
	case "stack":
		return "section-stack"
	case "timeline":
		return "section-timeline"
	case "split":
		return "section-split"
	default:
		return "section-grid"
	}
}

func resolveFormSubmissionType(form *models.Form, settings *models.FormSettingsDTO) string {
	if settings != nil && settings.SubmissionTarget != nil {
		target := strings.ToLower(strings.TrimSpace(*settings.SubmissionTarget))
		if target != "" {
			return target
		}
	}

	if settings != nil && settings.FormType != nil {
		formType := strings.ToLower(strings.TrimSpace(*settings.FormType))
		switch formType {
		case "leadership", "workforce", "testimonial", "event", "registration", "application", "contact", "general":
			return formType
		case "membership":
			return "member"
		}
	}

	if form != nil {
		title := strings.ToLower(strings.TrimSpace(form.Title))
		slug := ""
		if form.Slug != nil {
			slug = strings.ToLower(strings.TrimSpace(*form.Slug))
		}

		switch {
		case strings.Contains(title, "leadership"), strings.Contains(slug, "leadership"):
			return "leadership"
		case strings.Contains(title, "testimon"), strings.Contains(slug, "testimon"):
			return "testimonial"
		case strings.Contains(title, "member"), strings.Contains(title, "membership"), strings.Contains(slug, "member"):
			return "member"
		case strings.Contains(title, "workforce"), strings.Contains(slug, "workforce"):
			return "workforce"
		}
	}

	return "general"
}

func (h *FormHandler) renderPublicFormPageError(c *gin.Context, status int, title, message string) {
	html := "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>" + template.HTMLEscapeString(title) + "</title>" +
		"<style>body{margin:0;font-family:Segoe UI,Tahoma,Arial,sans-serif;background:#f4f7fb;color:#0f172a}main{max-width:760px;margin:80px auto;padding:32px;background:#fff;border:1px solid #dbe3ef;border-radius:24px;box-shadow:0 18px 50px rgba(15,23,42,.08)}h1{margin:0 0 12px;font-size:32px}p{margin:0;color:#475569;line-height:1.7}</style>" +
		"</head><body><main><h1>" + template.HTMLEscapeString(title) + "</h1><p>" + template.HTMLEscapeString(message) + "</p></main></body></html>"
	c.Data(status, "text/html; charset=utf-8", []byte(html))
}

const publicFormPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Form.Title}}</title>
  <style>
    :root {
      --ink: #0f172a;
      --muted: #475569;
      --line: #e2e8f0;
      --surface: #ffffff;
      --soft: #f8fafc;
      --brand: #0f4c81;
      --brand-soft: #e9f2f9;
      --gold: #d97706;
      --gold-dark: #b45309;
      --gold-soft: #fef3c7;
      --success: #166534;
      --danger: #b42318;
      --bg: radial-gradient(circle at 50% 0%, #edf3fb 0%, #f8fafc 100%);
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; color: var(--ink); background: var(--bg); -webkit-font-smoothing: antialiased; }
    .shell { max-width: 1140px; margin: 0 auto; padding: 36px 20px 64px; }
    .banner-box { width: 100%; max-height: 420px; border-radius: 24px; overflow: hidden; margin-bottom: 24px; box-shadow: 0 16px 40px rgba(15,23,42,.12); border: 1px solid rgba(226,232,240,0.8); background: #0f172a; }
    .banner-img { width: 100%; height: 100%; max-height: 420px; object-fit: cover; display: block; }
    .hero { background: linear-gradient(135deg, #0b1f3a 0%, #0f4c81 50%, #173860 100%); color: #fff; border-radius: 26px; padding: 36px; box-shadow: 0 20px 60px rgba(11,31,58,.22); position: relative; overflow: hidden; }
    .hero::after { content: ""; position: absolute; top: -60px; right: -60px; width: 220px; height: 220px; border-radius: 999px; background: radial-gradient(circle, rgba(217,119,6,0.2) 0%, transparent 70%); pointer-events: none; }
    .badge-tag { display: inline-flex; align-items: center; gap: 6px; padding: 5px 12px; border-radius: 999px; background: rgba(255,255,255,0.12); border: 1px solid rgba(255,255,255,0.2); font-size: 11px; font-weight: 800; letter-spacing: 0.08em; text-transform: uppercase; color: #fde68a; margin-bottom: 12px; }
    .hero h1 { margin: 0 0 10px; font-size: clamp(30px, 4.5vw, 44px); font-weight: 800; letter-spacing: -0.02em; line-height: 1.1; }
    .hero p { margin: 0; max-width: 760px; line-height: 1.7; color: rgba(255,255,255,.9); font-size: 15px; }
    .note { margin-top: 18px; padding: 14px 18px; border-radius: 16px; background: rgba(255,255,255,.12); border: 1px solid rgba(255,255,255,.2); font-size: 14px; line-height: 1.6; }
    .grid { display: grid; grid-template-columns: minmax(0, 1.05fr) minmax(320px, .95fr); gap: 24px; margin-top: 24px; align-items: start; }
    .card { background: var(--surface); border: 1px solid var(--line); border-radius: 24px; padding: 28px; box-shadow: 0 12px 35px rgba(15,23,42,.05); }
    h2 { margin: 0 0 14px; font-size: 22px; font-weight: 800; letter-spacing: -0.01em; color: var(--ink); }
    .muted { color: var(--muted); line-height: 1.65; font-size: 14px; }
    .bullet-list, .section-items { display: grid; gap: 14px; margin-top: 18px; }
    .bullet-item, .section-item { padding: 16px 18px; border-radius: 18px; background: var(--soft); border: 1px solid var(--line); }
    .section-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
    .section-stack { display: grid; gap: 14px; }
    .section-split { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
    .section-timeline { display: grid; gap: 14px; border-left: 2px solid var(--brand-soft); padding-left: 16px; }
    .eyebrow { margin: 0 0 6px; font-size: 11px; letter-spacing: .08em; text-transform: uppercase; color: var(--gold); font-weight: 800; }
    .section-item h3, .bullet-item h3 { margin: 0 0 6px; font-size: 17px; font-weight: 700; }
    .section-item a { color: var(--brand); font-weight: 700; text-decoration: none; }
    form { display: grid; gap: 18px; }
    .field { display: grid; gap: 8px; }
    .field-label { font-size: 13px; font-weight: 700; color: var(--ink); }
    .required { color: var(--danger); font-weight: 800; }
    .field-control { width: 100%; border: 1px solid #cbd5e1; border-radius: 14px; padding: 13px 16px; font: inherit; font-size: 14px; background: #fff; color: var(--ink); transition: border-color .15s ease, box-shadow .15s ease; }
    .field-control:focus { outline: none; border-color: var(--gold); box-shadow: 0 0 0 3px rgba(217, 119, 6, 0.15); }
    select.field-control { appearance: none; background-image: linear-gradient(45deg, transparent 50%, #64748b 50%), linear-gradient(135deg, #64748b 50%, transparent 50%); background-position: calc(100% - 20px) calc(1em + 1px), calc(100% - 14px) calc(1em + 1px); background-size: 6px 6px, 6px 6px; background-repeat: no-repeat; padding-right: 42px; }
    select.field-control option { color: #0f172a; background: #ffffff; }
    .textarea { min-height: 110px; resize: vertical; }
    .choice-group { display: grid; gap: 10px; }
    .choice { display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 14px; background: var(--soft); font-size: 14px; cursor: pointer; transition: background .15s ease, border-color .15s ease; }
    .choice:hover { background: #f1f5f9; border-color: #cbd5e1; }
    .choice.single { background: transparent; padding: 0; border: 0; }
    .field.invalid .field-control, .field.invalid .choice-group, .field.invalid .choice.single { border-color: #f04438; }
    .field-error { min-height: 18px; margin: -2px 0 0; color: var(--danger); font-size: 12px; font-weight: 600; }
    .actions { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; margin-top: 6px; }
    button { border: 0; border-radius: 999px; padding: 14px 26px; background: linear-gradient(135deg, #0b1f3a 0%, #0f4c81 100%); color: #fff; font: inherit; font-size: 14px; font-weight: 700; cursor: pointer; box-shadow: 0 8px 20px rgba(15,76,129,0.25); transition: transform .15s ease, box-shadow .15s ease, opacity .15s ease; }
    button:hover:not([disabled]) { transform: translateY(-1px); box-shadow: 0 12px 24px rgba(15,76,129,0.32); }
    button[disabled] { opacity: .6; cursor: wait; }
    .status { display: none; padding: 14px 16px; border-radius: 14px; font-size: 14px; line-height: 1.6; }
    .status.error { display: block; background: #fef3f2; color: var(--danger); border: 1px solid #fecdca; }
    .status.success { display: block; background: #ecfdf3; color: var(--success); border: 1px solid #abefc6; }
    .hidden-field { display: none !important; }
    .success-overlay { position: fixed; inset: 0; z-index: 50; display: none; align-items: center; justify-content: center; padding: 20px; background: rgba(2, 8, 23, 0.7); backdrop-filter: blur(6px); }
    .success-overlay.open { display: flex; }
    .success-modal { width: min(580px, 100%); border-radius: 26px; border: 1px solid #cbd5e1; background: #fff; box-shadow: 0 35px 80px rgba(15, 23, 42, 0.4); overflow: hidden; animation: modalPop 0.22s cubic-bezier(0.16, 1, 0.3, 1); }
    @keyframes modalPop { from { opacity: 0; transform: scale(0.95) translateY(10px); } to { opacity: 1; transform: scale(1) translateY(0); } }
    .success-head { padding: 28px 28px 24px; background: linear-gradient(135deg, #0b1f3a 0%, #0f4c81 100%); color: #fff; position: relative; }
    .success-head h3 { margin: 0; font-size: 24px; font-weight: 800; letter-spacing: -0.01em; line-height: 1.2; }
    .success-head p { margin: 8px 0 0; color: rgba(255, 255, 255, 0.9); font-size: 14px; line-height: 1.6; }
    .success-body { padding: 24px 28px 28px; display: grid; gap: 16px; }
    .lead-card { border: 1px solid #cbd5e1; border-radius: 18px; background: linear-gradient(180deg, #f8fafc, #eef4fb); padding: 16px; display: grid; grid-template-columns: 80px 1fr; gap: 14px; align-items: center; }
    .lead-avatar { width: 80px; height: 80px; border-radius: 999px; overflow: hidden; border: 2px solid #dbe3ef; background: #e2e8f0; display: flex; align-items: center; justify-content: center; font-size: 26px; font-weight: 800; color: #1e293b; text-transform: uppercase; }
    .lead-avatar img { width: 100%; height: 100%; object-fit: cover; display: block; }
    .lead-meta p { margin: 0; }
    .lead-name { font-size: 18px; font-weight: 800; color: #0f172a; }
    .lead-role { margin-top: 4px; font-size: 13px; color: #334155; font-weight: 700; }
    .prompt-box { border-radius: 14px; border: 1px solid #e2e8f0; background: #f8fafc; padding: 14px 16px; color: #334155; font-size: 14px; line-height: 1.6; }
    .dp-cta-box { border-radius: 18px; border: 1.5px solid rgba(217,119,6,0.3); background: linear-gradient(135deg, #fffbeb 0%, #fef3c7 100%); padding: 18px; text-align: center; }
    .dp-cta-box h4 { margin: 0 0 4px; font-size: 16px; font-weight: 800; color: #92400e; }
    .dp-cta-box p { margin: 0 0 14px; font-size: 13px; color: #b45309; line-height: 1.5; }
    .success-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 10px; }
    .link-btn { display: inline-flex; align-items: center; justify-content: center; gap: 8px; border-radius: 999px; padding: 12px 22px; text-decoration: none; font-size: 13px; font-weight: 700; background: var(--brand); color: #fff; transition: transform .15s ease; }
    .link-btn:hover { transform: translateY(-1px); }
    .gold-btn { background: linear-gradient(135deg, #d97706 0%, #b45309 100%); color: #fff; box-shadow: 0 8px 20px rgba(217,119,6,0.3); }
    .ghost-btn { border: 1px solid #cbd5e1; background: #fff; color: #0f172a; }
    @media (max-width: 980px) { .grid { grid-template-columns: 1fr; } }
    @media (max-width: 720px) {
      .section-grid, .section-split { grid-template-columns: 1fr; }
      .lead-card { grid-template-columns: 1fr; text-align: center; }
      .lead-avatar { margin: 0 auto; }
      .success-actions { flex-direction: column; }
      .success-actions .link-btn { width: 100%; text-align: center; }
    }
  </style>
</head>
<body>
  <div class="shell">
    {{if .CoverImageUrl}}
    <div class="banner-box">
      <img src="{{.CoverImageUrl}}" alt="{{.Form.Title}}" class="banner-img" onerror="this.parentElement.style.display='none'" />
    </div>
    {{end}}

    <section class="hero">
      <div class="badge-tag">✨ Official Registration</div>
      <h1>{{.Form.Title}}</h1>
      {{if .Form.Description}}<p>{{.Form.Description}}</p>{{end}}
      {{if and .Settings (hasText .Settings.FormHeaderNote)}}<div class="note">{{text .Settings.FormHeaderNote}}</div>{{end}}
    </section>

    <section class="grid">
      <div class="card">
        {{if and .Settings (hasText .Settings.IntroTitle)}}<h2>{{text .Settings.IntroTitle}}</h2>{{else}}<h2>Event Details</h2>{{end}}
        {{if and .Settings (hasText .Settings.IntroSubtitle)}}<p class="muted">{{text .Settings.IntroSubtitle}}</p>{{end}}

        {{if and .Settings .Settings.IntroBullets}}
          <div class="bullet-list">
            {{range $idx, $item := .Settings.IntroBullets}}
              <article class="bullet-item">
                <h3>{{$item}}</h3>
                {{if and $.Settings $.Settings.IntroBulletSubtexts}}
                  {{if lt $idx (len $.Settings.IntroBulletSubtexts)}}
                    <p class="muted">{{index $.Settings.IntroBulletSubtexts $idx}}</p>
                  {{end}}
                {{end}}
              </article>
            {{end}}
          </div>
        {{end}}

        {{if and .Settings .Settings.Sections}}
          {{range .Settings.Sections}}
            <section style="margin-top:24px;">
              <h2>{{.Title}}</h2>
              {{if hasText .Subtitle}}<p class="muted">{{text .Subtitle}}</p>{{end}}
              <div class="section-items {{layoutClass .Layout}}">
                {{range .Items}}
                  <article class="section-item">
                    {{if hasText .Eyebrow}}<p class="eyebrow">{{text .Eyebrow}}</p>{{end}}
                    <h3>{{.Title}}</h3>
                    {{if hasText .Body}}<p class="muted">{{text .Body}}</p>{{end}}
                    {{if and (hasText .LinkText) (hasText .LinkURL)}}<p style="margin:10px 0 0;"><a href="{{text .LinkURL}}" target="_blank" rel="noreferrer">{{text .LinkText}}</a></p>{{end}}
                  </article>
                {{end}}
              </div>
            </section>
          {{end}}
        {{end}}
      </div>

      <div class="card">
        <h2>Registration Form</h2>
        <p class="muted">Complete the details below. Required fields are marked with an asterisk.</p>
        <form id="public-form" novalidate>
          {{range .Fields}}
            <div class="field" data-field-key="{{.Key}}" data-field-type="{{.Type}}" data-required="{{.Required}}" {{if .VisibilityJSON}}data-visibility='{{.VisibilityJSON}}'{{end}}>
              {{renderFieldInput .}}
              <p class="field-error" data-error-for="{{.Key}}"></p>
            </div>
          {{end}}
          <div id="form-status" class="status"></div>
          <div class="actions">
            <button id="submit-button" type="submit">{{.SubmitLabel}}</button>
          </div>
        </form>
      </div>
    </section>
  </div>

  <div id="success-overlay" class="success-overlay" aria-hidden="true">
    <div class="success-modal" role="dialog" aria-modal="true" aria-labelledby="success-title">
      <div class="success-head">
        <h3 id="success-title">Registration Confirmed</h3>
        <p>Thank you for submitting your registration. Your details have been recorded.</p>
      </div>
      <div class="success-body">
        <div id="lead-card" class="lead-card" style="display:none;">
          <div id="lead-avatar" class="lead-avatar">WC</div>
          <div class="lead-meta">
            <p id="lead-name" class="lead-name">Leadership Applicant</p>
            <p id="lead-role" class="lead-role">Leadership</p>
          </div>
        </div>

        <div class="dp-cta-box" id="dp-cta-box">
          <h4>Personalized "I Will Be Attending" Status Flyer</h4>
          <p>Generate your custom badge and status flyer to share on WhatsApp, Instagram & Facebook!</p>
          <a id="dp-generator-btn" class="link-btn gold-btn" href="{{.DPGeneratorURL}}">
            ✨ Create Attendee DP & Flyer
          </a>
        </div>

        <div class="prompt-box" id="success-prompt">
          A confirmation response has been dispatched. You can now return to the homepage or customize your status flyer.
        </div>
        <div class="success-actions">
          <button id="success-close" type="button" class="link-btn ghost-btn">Close</button>
          <a id="home-link" class="link-btn" href="{{.HomeURL}}">Return to homepage</a>
        </div>
      </div>
    </div>
  </div>

  <script>
    const formElement = document.getElementById('public-form');
    const statusElement = document.getElementById('form-status');
    const submitButton = document.getElementById('submit-button');
    const successOverlay = document.getElementById('success-overlay');
    const successCloseButton = document.getElementById('success-close');
    const successHomeLink = document.getElementById('home-link');
    const leadCard = document.getElementById('lead-card');
    const leadAvatar = document.getElementById('lead-avatar');
    const leadName = document.getElementById('lead-name');
    const leadRole = document.getElementById('lead-role');
    const successPrompt = document.getElementById('success-prompt');
    const submitUrl = {{printf "%q" .SubmitURL}};
    const successMessage = {{printf "%q" .SuccessMessage}};
    const submissionType = {{printf "%q" .SubmissionType}};
    const homeUrl = {{printf "%q" .HomeURL}};
    if (successHomeLink && homeUrl) {
      successHomeLink.setAttribute('href', homeUrl);
    }

    function getFieldValue(fieldKey) {
      const checkboxes = Array.from(formElement.querySelectorAll('input[type="checkbox"][name="' + fieldKey + '"]'));
      if (checkboxes.length > 1) {
        return checkboxes.filter((input) => input.checked).map((input) => input.value);
      }
      if (checkboxes.length === 1) {
        return checkboxes[0].checked;
      }

      const radios = Array.from(formElement.querySelectorAll('input[type="radio"][name="' + fieldKey + '"]'));
      if (radios.length > 0) {
        const checked = radios.find((input) => input.checked);
        return checked ? checked.value : "";
      }

      const input = formElement.querySelector('[name="' + fieldKey + '"]');
      if (!input) return null;
      return input.value;
    }

    function normalizeComparable(value) {
      if (typeof value === 'string') return value.trim().toLowerCase();
      if (typeof value === 'number' || typeof value === 'boolean') return String(value).toLowerCase();
      return '';
    }

    function compareValue(source, target) {
      if (Array.isArray(source)) {
        return source.some((item) => compareValue(item, target));
      }
      if (Array.isArray(target)) {
        return target.some((item) => compareValue(source, item));
      }
      return normalizeComparable(source) === normalizeComparable(target);
    }

    function evaluateRule(rule, values) {
      const value = values[rule.fieldKey];
      if (value === undefined || value === null) return false;
      switch ((rule.operator || '').toLowerCase()) {
        case 'equals':
          return compareValue(value, rule.value);
        case 'not_equals':
          return !compareValue(value, rule.value);
        case 'in':
          return Array.isArray(rule.values) && rule.values.some((item) => compareValue(value, item));
        case 'not_in':
          return Array.isArray(rule.values) && !rule.values.some((item) => compareValue(value, item));
        default:
          return false;
      }
    }

    function updateVisibility() {
      const values = {};
      formElement.querySelectorAll('[data-field-key]').forEach((field) => {
        values[field.dataset.fieldKey] = getFieldValue(field.dataset.fieldKey);
      });

      formElement.querySelectorAll('[data-visibility]').forEach((field) => {
        let visible = true;
        try {
          const visibility = JSON.parse(field.dataset.visibility);
          if (visibility && Array.isArray(visibility.rules) && visibility.rules.length > 0) {
            const match = (visibility.match || 'all').toLowerCase();
            visible = match === 'any'
              ? visibility.rules.some((rule) => evaluateRule(rule, values))
              : visibility.rules.every((rule) => evaluateRule(rule, values));
          }
        } catch (_) {
          visible = true;
        }
        field.classList.toggle('hidden-field', !visible);
        field.querySelectorAll('input, select, textarea').forEach((input) => {
          input.disabled = !visible;
        });
      });
    }

    function collectValues() {
      const payload = {};
      formElement.querySelectorAll('[data-field-key]').forEach((field) => {
        if (field.classList.contains('hidden-field')) return;
        const key = field.dataset.fieldKey;
        payload[key] = getFieldValue(key);
      });
      return payload;
    }

    function setStatus(type, message) {
      statusElement.className = 'status ' + type;
      statusElement.textContent = message;
    }

    function setFieldError(field, message) {
      field.classList.toggle('invalid', Boolean(message));
      const error = field.querySelector('[data-error-for]');
      if (error) {
        error.textContent = message || '';
      }
    }

    function clearFieldErrors() {
      formElement.querySelectorAll('[data-field-key]').forEach((field) => setFieldError(field, ''));
    }

    function firstValue(values, keys) {
      for (const key of keys) {
        if (!(key in values)) continue;
        const value = values[key];
        if (typeof value === 'string' && value.trim()) return value.trim();
      }
      return '';
    }

    function initials(name) {
      const parts = String(name || '').trim().split(/\s+/).filter(Boolean);
      if (parts.length === 0) return 'WC';
      if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }

    function buildLeadershipCard(values) {
      const name = firstValue(values, ['full_name', 'fullName', 'name']) ||
        [firstValue(values, ['first_name', 'firstName', 'firstname']), firstValue(values, ['last_name', 'lastName', 'lastname'])].filter(Boolean).join(' ');
      const role = firstValue(values, ['role', 'leadershipRole', 'leadership_role', 'leadershipCategory']) || 'Leadership Applicant';
      const image = firstValue(values, ['imageUrl', 'image_url', 'image', 'photo', 'profileImage', 'profile_image']);
      return { name: name || 'Leadership Applicant', role, image };
    }

    function closeSuccessOverlay() {
      if (!successOverlay) return;
      successOverlay.classList.remove('open');
      successOverlay.setAttribute('aria-hidden', 'true');
    }

    function openSuccessOverlay(values) {
      if (!successOverlay) {
        setStatus('success', successMessage);
        return;
      }

      if (submissionType === 'leadership' && leadCard && leadAvatar && leadName && leadRole) {
        const card = buildLeadershipCard(values || {});
        leadName.textContent = card.name;
        leadRole.textContent = card.role || 'Leadership Applicant';
        if (card.image && /^https?:\/\//i.test(card.image)) {
          leadAvatar.innerHTML = '<img src="' + card.image.replace(/"/g, '&quot;') + '" alt="Leadership profile image" />';
        } else {
          leadAvatar.textContent = initials(card.name);
        }
        leadCard.style.display = 'grid';
        if (successPrompt) {
          successPrompt.textContent = 'Thank you for completing the Bio Data. Return back to homepage to continue.';
        }
      } else if (leadCard) {
        leadCard.style.display = 'none';
      }

      const dpBtn = document.getElementById('dp-generator-btn');
      if (dpBtn) {
        const attendeeName = firstValue(values || {}, ['full_name', 'fullName', 'name']) ||
          [firstValue(values || {}, ['first_name', 'firstName', 'firstname']), firstValue(values || {}, ['last_name', 'lastName', 'lastname'])].filter(Boolean).join(' ');
        if (attendeeName) {
          const base = {{printf "%q" .DPGeneratorURL}};
          dpBtn.href = base + (base.includes('?') ? '&' : '?') + 'name=' + encodeURIComponent(attendeeName);
        }
      }

      successOverlay.classList.add('open');
      successOverlay.setAttribute('aria-hidden', 'false');
    }

    function validateField(field) {
      if (field.classList.contains('hidden-field')) {
        setFieldError(field, '');
        return true;
      }

      const required = field.dataset.required === 'true';
      if (!required) {
        setFieldError(field, '');
        return true;
      }

      const key = field.dataset.fieldKey;
      const type = (field.dataset.fieldType || '').toLowerCase();
      const value = getFieldValue(key);

      let valid = true;
      if (type === 'checkbox') {
        if (Array.isArray(value)) {
          valid = value.length > 0;
        } else {
          valid = value === true;
        }
      } else if (type === 'radio' || type === 'select') {
        valid = typeof value === 'string' && value.trim() !== '';
      } else {
        valid = typeof value === 'string' && value.trim() !== '';
      }

      setFieldError(field, valid ? '' : 'This field is required.');
      return valid;
    }

    function validateVisibleFields() {
      let allValid = true;
      formElement.querySelectorAll('[data-field-key]').forEach((field) => {
        if (!validateField(field)) {
          allValid = false;
        }
      });
      return allValid;
    }

    formElement.addEventListener('input', updateVisibility);
    formElement.addEventListener('change', updateVisibility);
    formElement.addEventListener('input', clearFieldErrors);
    formElement.addEventListener('change', clearFieldErrors);
    updateVisibility();
    if (successCloseButton) {
      successCloseButton.addEventListener('click', closeSuccessOverlay);
    }
    if (successOverlay) {
      successOverlay.addEventListener('click', (event) => {
        if (event.target === successOverlay) closeSuccessOverlay();
      });
    }

    formElement.addEventListener('submit', async (event) => {
      event.preventDefault();
      submitButton.disabled = true;
      setStatus('', '');
      statusElement.className = 'status';

      try {
        clearFieldErrors();
        if (!validateVisibleFields()) {
          throw new Error('Please complete the required fields before submitting.');
        }

        const response = await fetch(submitUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ values: collectValues() }),
        });
        const body = await response.json().catch(() => ({}));
        if (!response.ok) {
          throw new Error(body.message || 'Unable to submit the form.');
        }

        const submittedValues = collectValues();
        formElement.reset();
        clearFieldErrors();
        updateVisibility();
        setStatus('success', successMessage);
        openSuccessOverlay(submittedValues);
      } catch (error) {
        const match = String(error && error.message || '').match(/field '([^']+)'/i);
        if (match) {
          const field = formElement.querySelector('[data-field-key="' + match[1] + '"]');
          if (field) {
            setFieldError(field, error.message);
          }
        }
        setStatus('error', error.message || 'Unable to submit the form.');
      } finally {
        submitButton.disabled = false;
      }
    });
  </script>
</body>
</html>`
