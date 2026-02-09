package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/exportpdf"
	"wisdomHouse-backend/pkg/utils"
)

// This DTO must match your JSON tags from ListSubmissions output.
// If your API uses different tags, change these json tags to match.
type exportSubmissionDTO struct {
	ID             string         `json:"id"`
	FormID         string         `json:"formId"`
	Name           string         `json:"name"`
	Email          string         `json:"email"`
	ContactNumber  string         `json:"contactNumber"`
	ContactAddress string         `json:"contactAddress"`
	CreatedAt      time.Time      `json:"createdAt"`
	Values         map[string]any `json:"values"`
}

// ExportAdminSubmissionsPDF generates an encrypted PDF of ALL submissions for a form.
// Password = authenticated user's email.
func (h *FormHandler) ExportAdminSubmissionsPDF(c *gin.Context) {
	formID := strings.TrimSpace(c.Param("id"))
	if formID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "missing form id")
		return
	}

	email := getAuthEmailFromContext(c)
	if email == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Load form
	form, err := h.svc.GetByID(formID)
	if err != nil || form == nil {
		utils.ErrorResponse(c, http.StatusNotFound, "form not found")
		return
	}

	// Load ALL submissions via paging loop
	const pageSize = 500
	page := 1
	var all []exportSubmissionDTO

	for {
		var start *time.Time
		var end *time.Time

		items, total, err := h.svc.ListSubmissions(formID, page, pageSize, start, end)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to load submissions")
			return
		}

		chunk, convErr := coerceToExportDTO(items)
		if convErr != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to prepare submissions for export")
			return
		}

		all = append(all, chunk...)

		if int64(len(all)) >= total || len(chunk) < pageSize {
			break
		}

		page++
		if page > 1_000_000 {
			break
		}
	}

	// Map into exportpdf.Submission
	subs := make([]exportpdf.Submission, 0, len(all))
	for _, r := range all {
		subs = append(subs, exportpdf.Submission{
			ID:             r.ID,
			Name:           r.Name,
			Email:          r.Email,
			ContactNumber:  r.ContactNumber,
			ContactAddress: r.ContactAddress,
			CreatedAt:      r.CreatedAt,
			Values:         r.Values,
		})
	}

	title := safeString(form.Title)

	pdfBytes, err := exportpdf.BuildSubmissionsPDF(title, subs)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to generate pdf")
		return
	}

	encrypted, err := exportpdf.EncryptPDF(pdfBytes, email)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to encrypt pdf")
		return
	}

	filename := sanitizeFilename(title) + "-submissions.pdf"

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", encrypted)
}

func getAuthEmailFromContext(c *gin.Context) string {
	for _, key := range []string{"email", "userEmail", "authEmail"} {
		if v, ok := c.Get(key); ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func coerceToExportDTO(items any) ([]exportSubmissionDTO, error) {
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var out []exportSubmissionDTO
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func safeString(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return "Form"
	}
	return s
}

func sanitizeFilename(title string) string {
	t := strings.TrimSpace(title)
	t = strings.ToLower(t)
	t = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == ' ':
			return r
		default:
			return -1
		}
	}, t)
	t = strings.Join(strings.Fields(t), "-")
	if t == "" {
		return "form"
	}
	return t
}
