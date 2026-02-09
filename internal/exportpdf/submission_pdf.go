package exportpdf

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// Submission is a minimal shape for PDF export.
type Submission struct {
	ID             string
	Name           string
	Email          string
	ContactNumber  string
	ContactAddress string
	CreatedAt      time.Time
	Values         map[string]any
}

// BuildSubmissionsPDF generates a readable PDF export.
func BuildSubmissionsPDF(formTitle string, submissions []Submission) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 14, 12)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Form Submissions Export", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 7, fmt.Sprintf("Form: %s", safeOneLine(formTitle)), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 7, fmt.Sprintf("Exported: %s", time.Now().Format(time.RFC1123)), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Determine stable set of dynamic keys from Values
	valueKeys := collectValueKeys(submissions)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, fmt.Sprintf("Total submissions: %d", len(submissions)), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	for idx, s := range submissions {
		pdf.SetFont("Arial", "B", 12)
		pdf.CellFormat(
			0,
			7,
			fmt.Sprintf("%d) %s", idx+1, safeOneLine(firstNonEmpty(s.Name, s.Email, "Anonymous"))),
			"",
			1,
			"L",
			false,
			0,
			"",
		)

		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 5, fmt.Sprintf(
			"Submitted: %s\nEmail: %s\nPhone: %s\nAddress: %s\nSubmission ID: %s",
			s.CreatedAt.Format(time.RFC1123),
			safeOneLine(s.Email),
			safeOneLine(s.ContactNumber),
			safeOneLine(s.ContactAddress),
			safeOneLine(s.ID),
		), "", "L", false)
		pdf.Ln(1)

		// Values section
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 6, "Responses:", "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)

		if len(valueKeys) == 0 || len(s.Values) == 0 {
			pdf.CellFormat(0, 5, "—", "", 1, "L", false, 0, "")
		} else {
			for _, k := range valueKeys {
				v, ok := s.Values[k]
				if !ok {
					continue
				}
				pdf.MultiCell(0, 5, fmt.Sprintf("• %s: %s", safeOneLine(k), safeOneLine(fmt.Sprint(v))), "", "L", false)
			}
		}

		pdf.Ln(2)
		pdf.Line(12, pdf.GetY(), 198, pdf.GetY())
		pdf.Ln(3)
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func collectValueKeys(subs []Submission) []string {
	set := map[string]struct{}{}
	for _, s := range subs {
		for k := range s.Values {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			set[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func safeOneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}
