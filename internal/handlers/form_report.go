package handlers

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

func (h *FormHandler) GetAdminFormReportLink(c *gin.Context) {
	formID := strings.TrimSpace(c.Param("id"))
	if formID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "missing form id")
		return
	}

	link, err := h.svc.GetOrCreateReportLink(formID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Report link ready", link)
}

func (h *FormHandler) GetPublicFormReportData(c *gin.Context) {
	payload, err := h.loadPublicFormReport(c)
	if err != nil {
		h.handlePublicReportError(c, err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Report loaded", payload)
}

func (h *FormHandler) ViewPublicFormReport(c *gin.Context) {
	payload, err := h.loadPublicFormReport(c)
	if err != nil {
		h.renderPublicReportError(c, err)
		return
	}

	pageData := struct {
		Payload        *models.PublicFormReportPayload
		PageLabel      string
		HasPrev        bool
		HasNext        bool
		PrevURL        string
		NextURL        string
		GeneratedLabel string
	}{
		Payload:        payload,
		PageLabel:      buildReportPageLabel(payload.Page, payload.TotalPages),
		HasPrev:        payload.Page > 1,
		HasNext:        payload.TotalPages > 0 && payload.Page < payload.TotalPages,
		PrevURL:        buildReportPaginationURL(c, payload.Page-1, payload.Limit),
		NextURL:        buildReportPaginationURL(c, payload.Page+1, payload.Limit),
		GeneratedLabel: formatReportTime(payload.GeneratedAt),
	}

	tpl := template.Must(template.New("public-form-report").Funcs(template.FuncMap{
		"formatTime":        formatReportTime,
		"hasValue":          hasPointerValue,
		"renderPointer":     renderPointer,
		"displayRegistrant": displayRegistrant,
	}).Parse(publicFormReportHTML))

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	_ = tpl.Execute(c.Writer, pageData)
}

func (h *FormHandler) ExportPublicFormReportPDF(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "missing form slug")
		return
	}

	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	filename, pdfBytes, err := h.svc.BuildPublicReportPDF(slug, reportAccessTokenFromRequest(c), start, end)
	if err != nil {
		h.handlePublicReportError(c, err)
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func (h *FormHandler) loadPublicFormReport(c *gin.Context) (*models.PublicFormReportPayload, error) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		return nil, service.ErrFormReportAccessDenied
	}

	start, end, err := parseTimeRange(c)
	if err != nil {
		return nil, err
	}

	page := parseIntClamp(c.DefaultQuery("page", "1"), 1, 1_000_000)
	limit := parseIntClamp(c.DefaultQuery("limit", "25"), 1, 100)

	return h.svc.GetPublicReport(slug, reportAccessTokenFromRequest(c), page, limit, start, end)
}

func (h *FormHandler) handlePublicReportError(c *gin.Context, err error) {
	switch err {
	case gorm.ErrRecordNotFound, service.ErrFormReportAccessDenied:
		utils.ErrorResponse(c, http.StatusNotFound, "report not found")
	default:
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
	}
}

func (h *FormHandler) renderPublicReportError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	title := "Unable to load report"
	message := err.Error()

	switch err {
	case gorm.ErrRecordNotFound, service.ErrFormReportAccessDenied:
		status = http.StatusNotFound
		title = "Report not available"
		message = "This report link is invalid, unavailable, or has been revoked."
	}

	html := "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>" + template.HTMLEscapeString(title) + "</title>" +
		"<style>body{margin:0;font-family:Segoe UI,Tahoma,Arial,sans-serif;background:#f4f6fb;color:#0f172a}main{max-width:720px;margin:80px auto;padding:32px;background:#fff;border:1px solid #dbe3ef;border-radius:24px;box-shadow:0 18px 50px rgba(15,23,42,.08)}h1{margin:0 0 12px;font-size:32px}p{margin:0;color:#475569;line-height:1.7}</style>" +
		"</head><body><main><h1>" + template.HTMLEscapeString(title) + "</h1><p>" + template.HTMLEscapeString(message) + "</p></main></body></html>"

	c.Data(status, "text/html; charset=utf-8", []byte(html))
}

func reportAccessTokenFromRequest(c *gin.Context) string {
	token := strings.TrimSpace(c.Query("access"))
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	return token
}

func buildReportPaginationURL(c *gin.Context, page, limit int) string {
	if page < 1 {
		page = 1
	}

	q := url.Values{}
	for key, values := range c.Request.URL.Query() {
		if key == "page" || key == "limit" {
			continue
		}
		for _, value := range values {
			q.Add(key, value)
		}
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))

	encoded := q.Encode()
	if encoded == "" {
		return c.Request.URL.Path
	}
	return c.Request.URL.Path + "?" + encoded
}

func buildReportPageLabel(page, totalPages int) string {
	if totalPages <= 0 {
		return "Page 1 of 1"
	}
	return "Page " + strconv.Itoa(page) + " of " + strconv.Itoa(totalPages)
}

func formatReportTime(value any) string {
	switch v := value.(type) {
	case time.Time:
		return v.UTC().Format("02 Jan 2006 15:04 UTC")
	case *time.Time:
		if v == nil {
			return ""
		}
		return v.UTC().Format("02 Jan 2006 15:04 UTC")
	default:
		return ""
	}
}

func hasPointerValue(v *string) bool {
	return v != nil && strings.TrimSpace(*v) != ""
}

func renderPointer(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func displayRegistrant(item models.FormReportSubmission) string {
	if item.Name != nil && strings.TrimSpace(*item.Name) != "" {
		return strings.TrimSpace(*item.Name)
	}
	if item.Email != nil && strings.TrimSpace(*item.Email) != "" {
		return strings.TrimSpace(*item.Email)
	}
	return "Anonymous registrant"
}

const publicFormReportHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Payload.FormTitle}} Report</title>
  <style>
    :root {
      --ink: #0f172a;
      --muted: #475569;
      --line: #dbe3ef;
      --surface: #ffffff;
      --surface-soft: #f8fafc;
      --accent: #0f4c81;
      --accent-soft: #e8f1f8;
      --bg: linear-gradient(180deg, #eef4fb 0%, #f7fafc 100%);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", Tahoma, Arial, sans-serif;
      color: var(--ink);
      background: var(--bg);
    }
    .shell {
      max-width: 1180px;
      margin: 0 auto;
      padding: 40px 20px 64px;
    }
    .hero {
      background: rgba(15, 76, 129, 0.94);
      color: #fff;
      border-radius: 28px;
      padding: 32px;
      box-shadow: 0 24px 70px rgba(15, 23, 42, 0.18);
    }
    .eyebrow {
      margin: 0 0 10px;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      font-size: 12px;
      opacity: 0.82;
    }
    h1 {
      margin: 0 0 10px;
      font-size: clamp(32px, 5vw, 46px);
      line-height: 1.05;
    }
    .hero p {
      margin: 0;
      max-width: 760px;
      line-height: 1.7;
      color: rgba(255, 255, 255, 0.9);
    }
    .hero-actions {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin-top: 22px;
    }
    .hero-actions a {
      text-decoration: none;
      border-radius: 999px;
      padding: 12px 18px;
      font-weight: 700;
    }
    .hero-actions .primary {
      background: #fff;
      color: var(--accent);
    }
    .hero-actions .secondary {
      background: rgba(255, 255, 255, 0.18);
      color: #fff;
      border: 1px solid rgba(255, 255, 255, 0.25);
    }
    .meta {
      display: flex;
      flex-wrap: wrap;
      gap: 14px;
      margin-top: 18px;
      color: rgba(255, 255, 255, 0.85);
      font-size: 14px;
    }
    .grid {
      display: grid;
      gap: 20px;
      grid-template-columns: repeat(12, minmax(0, 1fr));
      margin-top: 24px;
    }
    .card {
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: 24px;
      padding: 22px;
      box-shadow: 0 12px 40px rgba(15, 23, 42, 0.06);
    }
    .stats {
      grid-column: span 12;
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 16px;
    }
    .stat {
      padding: 18px;
      border-radius: 20px;
      background: var(--surface-soft);
      border: 1px solid var(--line);
    }
    .stat-label {
      margin: 0 0 8px;
      color: var(--muted);
      font-size: 13px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
    }
    .stat-value {
      margin: 0;
      font-size: 30px;
      font-weight: 800;
      line-height: 1.1;
    }
    .latest {
      grid-column: span 4;
    }
    .submissions {
      grid-column: span 8;
    }
    h2 {
      margin: 0 0 16px;
      font-size: 22px;
    }
    .list {
      display: grid;
      gap: 14px;
    }
    .pill {
      display: inline-block;
      padding: 6px 10px;
      border-radius: 999px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 12px;
      font-weight: 700;
    }
    .latest-item {
      border: 1px solid var(--line);
      border-radius: 18px;
      padding: 16px;
      background: #fff;
    }
    .latest-item h3,
    .submission-card h3 {
      margin: 0 0 6px;
      font-size: 18px;
    }
    .muted {
      color: var(--muted);
      font-size: 14px;
      line-height: 1.6;
    }
    .submission-card {
      border: 1px solid var(--line);
      border-radius: 20px;
      overflow: hidden;
      background: #fff;
    }
    .submission-card summary {
      list-style: none;
      cursor: pointer;
      padding: 18px 20px;
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: flex-start;
    }
    .submission-card summary::-webkit-details-marker {
      display: none;
    }
    .submission-meta {
      color: var(--muted);
      font-size: 13px;
      text-align: right;
      min-width: 160px;
    }
    .submission-body {
      padding: 0 20px 20px;
      border-top: 1px solid var(--line);
      background: var(--surface-soft);
    }
    .kv {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
      margin-top: 16px;
    }
    .kv div {
      padding: 12px 14px;
      border-radius: 16px;
      background: #fff;
      border: 1px solid var(--line);
    }
    .kv strong {
      display: block;
      margin-bottom: 6px;
      font-size: 13px;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .pager {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      margin-top: 18px;
    }
    .pager a {
      text-decoration: none;
      color: var(--accent);
      font-weight: 700;
    }
    @media (max-width: 980px) {
      .latest, .submissions, .stats {
        grid-column: span 12;
      }
      .stats {
        grid-template-columns: 1fr;
      }
    }
    @media (max-width: 720px) {
      .submission-card summary {
        flex-direction: column;
      }
      .submission-meta {
        text-align: left;
        min-width: 0;
      }
      .kv {
        grid-template-columns: 1fr;
      }
    }
  </style>
</head>
<body>
  <div class="shell">
    <section class="hero">
      <p class="eyebrow">{{.Payload.BrandName}} Registration Report</p>
      <h1>{{.Payload.FormTitle}}</h1>
      {{if hasValue .Payload.FormDescription}}<p>{{renderPointer .Payload.FormDescription}}</p>{{end}}
      <div class="hero-actions">
        <a class="primary" href="{{.Payload.ExportPDFURL}}">Download PDF</a>
        <a class="secondary" href="{{.Payload.ReportDataURL}}">Open JSON Data</a>
      </div>
      <div class="meta">
        <span>Generated: {{.GeneratedLabel}}</span>
        <span>Slug: {{.Payload.Slug}}</span>
        <span>Total records: {{.Payload.Total}}</span>
      </div>
    </section>

    <section class="grid">
      <div class="stats">
        <article class="stat">
          <p class="stat-label">Total Registrations</p>
          <p class="stat-value">{{.Payload.Summary.TotalSubmissions}}</p>
        </article>
        <article class="stat">
          <p class="stat-label">Latest Registration</p>
          <p class="stat-value">{{if .Payload.Summary.LatestSubmissionAt}}{{formatTime .Payload.Summary.LatestSubmissionAt}}{{else}}No entries yet{{end}}</p>
        </article>
        <article class="stat">
          <p class="stat-label">Current View</p>
          <p class="stat-value">{{.PageLabel}}</p>
        </article>
      </div>

      <article class="card latest">
        <h2>Latest People Registered</h2>
        <div class="list">
          {{if .Payload.LatestRegistrations}}
            {{range .Payload.LatestRegistrations}}
              <div class="latest-item">
                <span class="pill">Latest</span>
                <h3>{{displayRegistrant .}}</h3>
                <p class="muted">{{if hasValue .Email}}{{renderPointer .Email}}{{else}}No email provided{{end}}</p>
                <p class="muted">{{formatTime .CreatedAt}}</p>
                {{if hasValue .RegistrationCode}}<p class="muted">Code: {{renderPointer .RegistrationCode}}</p>{{end}}
              </div>
            {{end}}
          {{else}}
            <p class="muted">No registrations have been recorded yet.</p>
          {{end}}
        </div>
      </article>

      <article class="card submissions">
        <h2>Submission Details</h2>
        <div class="list">
          {{if .Payload.Submissions}}
            {{range .Payload.Submissions}}
              <details class="submission-card">
                <summary>
                  <div>
                    <h3>{{displayRegistrant .}}</h3>
                    <p class="muted">
                      {{if hasValue .Email}}{{renderPointer .Email}}{{else}}No email provided{{end}}
                      {{if hasValue .ContactNumber}} | {{renderPointer .ContactNumber}}{{end}}
                    </p>
                  </div>
                  <div class="submission-meta">
                    <div>{{formatTime .CreatedAt}}</div>
                    {{if hasValue .RegistrationCode}}<div>Code: {{renderPointer .RegistrationCode}}</div>{{end}}
                  </div>
                </summary>
                <div class="submission-body">
                  <div class="kv">
                    {{range .Fields}}
                      <div>
                        <strong>{{.Label}}</strong>
                        <span>{{.Value}}</span>
                      </div>
                    {{end}}
                  </div>
                </div>
              </details>
            {{end}}
          {{else}}
            <p class="muted">No submissions matched the current report view.</p>
          {{end}}
        </div>
        <div class="pager">
          <div>{{.PageLabel}}</div>
          <div>
            {{if .HasPrev}}<a href="{{.PrevURL}}">Previous</a>{{end}}
            {{if and .HasPrev .HasNext}}<span class="muted"> | </span>{{end}}
            {{if .HasNext}}<a href="{{.NextURL}}">Next</a>{{end}}
          </div>
        </div>
      </article>
    </section>
  </div>
</body>
</html>`
