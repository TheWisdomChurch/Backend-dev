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
	data := publicFormPageData{
		Form:           payload.Form,
		Settings:       settings,
		Fields:         buildPublicFormFieldViews(payload.Form.Fields),
		SubmitURL:      "/api/v1/forms/" + slug + "/submissions",
		SubmitLabel:    "Submit Registration",
		SuccessMessage: "Registration submitted successfully.",
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
      --line: #dbe3ef;
      --surface: #ffffff;
      --soft: #f8fafc;
      --brand: #0f4c81;
      --brand-soft: #e9f2f9;
      --success: #166534;
      --danger: #b42318;
      --bg: linear-gradient(180deg, #eef4fb 0%, #f9fbfd 100%);
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: "Segoe UI", Tahoma, Arial, sans-serif; color: var(--ink); background: var(--bg); }
    .shell { max-width: 1180px; margin: 0 auto; padding: 40px 20px 64px; }
    .hero { background: rgba(15, 76, 129, 0.96); color: #fff; border-radius: 28px; padding: 34px; box-shadow: 0 24px 70px rgba(15,23,42,.18); }
    .hero h1 { margin: 0 0 10px; font-size: clamp(32px, 5vw, 46px); line-height: 1.05; }
    .hero p { margin: 0; max-width: 760px; line-height: 1.75; color: rgba(255,255,255,.9); }
    .note { margin-top: 18px; padding: 14px 16px; border-radius: 18px; background: rgba(255,255,255,.14); border: 1px solid rgba(255,255,255,.2); }
    .grid { display: grid; grid-template-columns: minmax(0, 1.05fr) minmax(320px, .95fr); gap: 24px; margin-top: 26px; align-items: start; }
    .card { background: var(--surface); border: 1px solid var(--line); border-radius: 24px; padding: 24px; box-shadow: 0 12px 40px rgba(15,23,42,.06); }
    h2 { margin: 0 0 14px; font-size: 24px; }
    .muted { color: var(--muted); line-height: 1.7; }
    .bullet-list, .section-items { display: grid; gap: 14px; margin-top: 18px; }
    .bullet-item, .section-item { padding: 16px 18px; border-radius: 18px; background: var(--soft); border: 1px solid var(--line); }
    .section-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
    .section-stack { display: grid; gap: 14px; }
    .section-split { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
    .section-timeline { display: grid; gap: 14px; border-left: 2px solid var(--brand-soft); padding-left: 16px; }
    .eyebrow { margin: 0 0 6px; font-size: 12px; letter-spacing: .08em; text-transform: uppercase; color: var(--brand); font-weight: 700; }
    .section-item h3, .bullet-item h3 { margin: 0 0 6px; font-size: 18px; }
    .section-item a { color: var(--brand); font-weight: 700; text-decoration: none; }
    form { display: grid; gap: 18px; }
    .field { display: grid; gap: 10px; }
    .field-label { font-size: 14px; font-weight: 700; color: var(--ink); }
    .required { color: var(--danger); }
    .field-control { width: 100%; border: 1px solid #cbd5e1; border-radius: 16px; padding: 14px 16px; font: inherit; background: #fff; color: var(--ink); }
    .textarea { min-height: 120px; resize: vertical; }
    .choice-group { display: grid; gap: 12px; }
    .choice { display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 16px; background: var(--soft); }
    .choice.single { background: transparent; padding: 0; border: 0; }
    .field.invalid .field-control, .field.invalid .choice-group, .field.invalid .choice.single { border-color: #f04438; }
    .field-error { min-height: 18px; margin: -2px 0 0; color: var(--danger); font-size: 13px; }
    .actions { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
    button { border: 0; border-radius: 999px; padding: 14px 22px; background: var(--brand); color: #fff; font: inherit; font-weight: 700; cursor: pointer; }
    button[disabled] { opacity: .6; cursor: wait; }
    .status { display: none; padding: 14px 16px; border-radius: 16px; font-size: 14px; line-height: 1.6; }
    .status.error { display: block; background: #fef3f2; color: var(--danger); border: 1px solid #fecdca; }
    .status.success { display: block; background: #ecfdf3; color: var(--success); border: 1px solid #abefc6; }
    .hidden-field { display: none !important; }
    @media (max-width: 980px) { .grid { grid-template-columns: 1fr; } }
    @media (max-width: 720px) { .section-grid, .section-split { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
  <div class="shell">
    <section class="hero">
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

  <script>
    const formElement = document.getElementById('public-form');
    const statusElement = document.getElementById('form-status');
    const submitButton = document.getElementById('submit-button');
    const submitUrl = {{printf "%q" .SubmitURL}};
    const successMessage = {{printf "%q" .SuccessMessage}};

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

    function evaluateRule(rule, values) {
      const value = values[rule.fieldKey];
      if (value === undefined || value === null) return false;
      switch ((rule.operator || '').toLowerCase()) {
        case 'equals':
          return String(value) === String(rule.value);
        case 'not_equals':
          return String(value) !== String(rule.value);
        case 'in':
          return Array.isArray(rule.values) && rule.values.map(String).includes(String(value));
        case 'not_in':
          return Array.isArray(rule.values) && !rule.values.map(String).includes(String(value));
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

        formElement.reset();
        clearFieldErrors();
        updateVisibility();
        setStatus('success', successMessage);
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
