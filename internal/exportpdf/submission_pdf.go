package exportpdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jung-kurt/gofpdf"
)

// Fonts
//go:embed fonts/DejaVuSans.ttf
var dejavuSans []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var dejavuSansBold []byte

// Logo
// BEST PRACTICE: use PNG (assets/logo.png).
// If you keep webp, gofpdf won't render it; this code will skip it safely.
//go:embed assets/logo.webp
var logoBytes []byte

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

func BuildSubmissionsPDF(formTitle string, submissions []Submission) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")

	// Content margins (inside header/footer)
	const (
		marginL = 12.0
		marginR = 12.0

		// These margins MUST account for header + footer space
		marginTop    = 30.0
		marginBottom = 18.0
	)

	pdf.SetMargins(marginL, marginTop, marginR)
	pdf.SetAutoPageBreak(true, marginBottom)

	// Unicode fonts (REGISTRATION FIX)
	// Important: register regular and bold as separate "families" so B works reliably.
	pdf.AddUTF8FontFromBytes("DejaVu", "", dejavuSans)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", dejavuSansBold)

	// Register logo in-memory (no file system dependency)
	logoName, err := registerLogo(pdf)
	if err != nil {
		return nil, err
	}

	exportedAt := time.Now().UTC()

	// Header / Footer
	pdf.SetHeaderFunc(func() {
		drawHeader(pdf, logoName, formTitle, exportedAt)
		drawContentFrame(pdf) // subtle border framing the content area
	})

	pdf.SetFooterFunc(func() {
		drawFooter(pdf)
	})

	pdf.AliasNbPages("")
	pdf.AddPage()

	// Title block
	pdf.SetFont("DejaVu", "B", 14)
	pdf.CellFormat(0, 8, "Form Submissions Export", "", 1, "L", false, 0, "")
	pdf.SetFont("DejaVu", "", 10.5)
	pdf.CellFormat(0, 6, fmt.Sprintf("Total submissions: %d", len(submissions)), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Determine stable set of keys from Values (common keys first)
	valueKeys := collectValueKeys(submissions)

	for idx, s := range submissions {
		name, email := normalizeNameEmail(s.Name, s.Email, s.Values)
		displayTitle := firstNonEmpty(name, email, "Anonymous")

		// Submission heading
		pdf.SetFont("DejaVu", "B", 11)
		pdf.CellFormat(0, 6.5, fmt.Sprintf("%d) %s", idx+1, safeOneLine(displayTitle)), "", 1, "L", false, 0, "")

		pdf.SetFont("DejaVu", "", 9.5)
		pdf.CellFormat(0, 5, fmt.Sprintf("Submitted: %s", s.CreatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 MST")), "", 1, "L", false, 0, "")
		pdf.CellFormat(0, 5, fmt.Sprintf("Submission ID: %s", safeOneLine(s.ID)), "", 1, "L", false, 0, "")
		pdf.Ln(2)

		// Professional table
		rows := buildRows(s, name, email, valueKeys)
		drawKeyValueTable(pdf, rows)

		pdf.Ln(4)

		// Soft divider between submissions
		y := pdf.GetY()
		pdf.SetDrawColor(220, 220, 220)
		pdf.Line(marginL, y, 210-marginR, y)
		pdf.SetDrawColor(0, 0, 0)
		pdf.Ln(4)
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

/* =========================
   Header / Footer / Frame
========================= */

func registerLogo(pdf *gofpdf.Fpdf) (string, error) {
	if len(logoBytes) == 0 {
		return "", nil
	}

	// Detect image type from signature
	imgType := detectImageType(logoBytes)
	if imgType == "" {
		return "", nil
	}

	// NOTE: gofpdf supports PNG/JPG/GIF. It does NOT support WEBP.
	// If you embed .webp, we skip it safely (PDF still generates).
	if imgType == "WEBP" {
		return "", nil
	}

	name := "brand_logo"
	opt := gofpdf.ImageOptions{ImageType: imgType, ReadDpi: true}
	pdf.RegisterImageOptionsReader(name, opt, bytes.NewReader(logoBytes))
	return name, nil
}

func detectImageType(b []byte) string {
	if len(b) < 12 {
		return ""
	}

	// PNG signature
	if bytes.HasPrefix(b, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "PNG"
	}

	// JPEG signature
	if b[0] == 0xFF && b[1] == 0xD8 {
		return "JPG"
	}

	// GIF signature
	if bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")) {
		return "GIF"
	}

	// WEBP signature: "RIFF....WEBP"
	if bytes.HasPrefix(b, []byte("RIFF")) && bytes.HasPrefix(b[8:], []byte("WEBP")) {
		return "WEBP"
	}

	return ""
}

func drawHeader(pdf *gofpdf.Fpdf, logoName string, formTitle string, exportedAt time.Time) {
	const (
		headerTop = 10.0
		leftX     = 12.0
		rightX    = 198.0
	)

	// Logo
	if logoName != "" {
		logoW, logoH := 18.0, 18.0
		pdf.ImageOptions(logoName, leftX, headerTop, logoW, logoH, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
	}

	// Brand + Form title
	pdf.SetXY(leftX+22, headerTop)
	pdf.SetFont("DejaVu", "B", 11)
	pdf.CellFormat(0, 5.5, "The Wisdom House Church", "", 1, "L", false, 0, "")
	pdf.SetX(leftX + 22)
	pdf.SetFont("DejaVu", "", 9.5)
	pdf.CellFormat(0, 5.0, fmt.Sprintf("Form: %s", safeOneLine(formTitle)), "", 1, "L", false, 0, "")

	// Right side meta
	pdf.SetXY(120, headerTop)
	pdf.SetFont("DejaVu", "", 9)
	pdf.CellFormat(rightX-120, 5.0, "Exported (UTC):", "", 2, "R", false, 0, "")
	pdf.SetFont("DejaVu", "B", 9)
	pdf.CellFormat(rightX-120, 5.0, exportedAt.Format("02 Jan 2006 15:04"), "", 0, "R", false, 0, "")

	// Header separator line
	pdf.SetDrawColor(230, 230, 230)
	pdf.Line(leftX, 28.5, rightX, 28.5)
	pdf.SetDrawColor(0, 0, 0)
}

func drawFooter(pdf *gofpdf.Fpdf) {
	leftX := 12.0
	rightX := 198.0

	pdf.SetY(-14)
	pdf.SetDrawColor(230, 230, 230)
	pdf.Line(leftX, pdf.GetY(), rightX, pdf.GetY())
	pdf.SetDrawColor(0, 0, 0)

	pdf.SetY(-11.5)
	pdf.SetFont("DejaVu", "", 8.5)
	pdf.CellFormat(0, 5, "Confidential — For internal use only", "", 0, "L", false, 0, "")

	pdf.SetY(-11.5)
	pdf.CellFormat(0, 5, fmt.Sprintf("Page %d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
}

func drawContentFrame(pdf *gofpdf.Fpdf) {
	pageW, pageH := pdf.GetPageSize()
	_ = pageH

	left, top, right, bottom := pdf.GetMargins()
	x := left
	y := top - 6
	w := pageW - left - right
	h := pageH - top - bottom + 4

	pdf.SetDrawColor(242, 242, 242)
	pdf.Rect(x, y, w, h, "")
	pdf.SetDrawColor(0, 0, 0)
}

/* =========================
   Table + formatting logic
========================= */

type kvRow struct {
	Key   string
	Value string
}

func buildRows(s Submission, name, email string, dynamicKeys []string) []kvRow {
	rows := make([]kvRow, 0, 12)

	if v := safeOneLine(name); v != "" {
		rows = append(rows, kvRow{Key: "Full name", Value: v})
	}
	if v := safeOneLine(email); v != "" {
		rows = append(rows, kvRow{Key: "Email", Value: v})
	}

	phone := normalizePhone(firstNonEmpty(s.ContactNumber, valueFromKeys(s.Values, "phone", "contact_number", "field_4")))
	if phone != "" {
		rows = append(rows, kvRow{Key: "Phone", Value: phone})
	}

	addr := safeOneLine(firstNonEmpty(s.ContactAddress, valueFromKeys(s.Values, "address", "contact_address")))
	if addr != "" {
		rows = append(rows, kvRow{Key: "Address", Value: addr})
	}

	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[strings.ToLower(r.Key)] = struct{}{}
	}

	for _, k := range dynamicKeys {
		if s.Values == nil {
			continue
		}
		v, ok := s.Values[k]
		if !ok {
			continue
		}

		label := prettifyKey(k)
		if _, exists := seen[strings.ToLower(label)]; exists {
			continue
		}

		value := normalizeValueString(k, v)
		if strings.TrimSpace(value) == "" {
			continue
		}

		rows = append(rows, kvRow{Key: label, Value: value})
	}

	if len(rows) == 0 {
		rows = append(rows, kvRow{Key: "Responses", Value: "—"})
	}
	return rows
}

func drawKeyValueTable(pdf *gofpdf.Fpdf, rows []kvRow) {
	pageW, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	usableW := pageW - left - right

	keyW := 52.0
	valW := usableW - keyW
	lineH := 5.2

	// Header row
	pdf.SetFont("DejaVu", "B", 9.8)
	pdf.SetFillColor(245, 245, 245)
	pdf.SetDrawColor(230, 230, 230)
	pdf.CellFormat(keyW, 7, "Field", "1", 0, "L", true, 0, "")
	pdf.CellFormat(valW, 7, "Response", "1", 1, "L", true, 0, "")
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFont("DejaVu", "", 9.6)

	for _, r := range rows {
		key := safeOneLine(r.Key)
		val := safeMultiLine(r.Value)

		valLines := pdf.SplitLines([]byte(val), valW-3)
		h := float64(len(valLines))*lineH + 3
		if h < 7 {
			h = 7
		}

		x := pdf.GetX()
		y := pdf.GetY()

		// Key cell
		pdf.SetDrawColor(230, 230, 230)
		pdf.Rect(x, y, keyW, h, "")
		pdf.SetXY(x+1.5, y+1.5)
		pdf.MultiCell(keyW-3, lineH, key, "", "L", false)

		// Value cell
		pdf.SetXY(x+keyW, y)
		pdf.Rect(x+keyW, y, valW, h, "")
		pdf.SetXY(x+keyW+1.5, y+1.5)
		pdf.MultiCell(valW-3, lineH, val, "", "L", false)

		pdf.SetXY(x, y+h)
	}

	pdf.SetDrawColor(0, 0, 0)
}

/* =========================
   Key collection / cleaning
========================= */

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

	priority := map[string]int{
		"full_name": 0,
		"name":      1,
		"email":     2,
		"phone":     3,
		"field_4":   3,
		"address":   4,
		"field_3":   10,
		"field_5":   11,
		"field_6":   12,
	}

	sort.Slice(keys, func(i, j int) bool {
		ai := strings.ToLower(keys[i])
		aj := strings.ToLower(keys[j])
		pi, okI := priority[ai]
		pj, okJ := priority[aj]
		if okI && okJ && pi != pj {
			return pi < pj
		}
		if okI != okJ {
			return okI
		}
		return ai < aj
	})

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

func safeMultiLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.Join(strings.Fields(lines[i]), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func looksLikeEmail(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return emailRe.MatchString(s)
}

func normalizeNameEmail(name, email string, values map[string]any) (string, string) {
	name = safeOneLine(name)
	email = safeOneLine(email)

	if email == "" {
		email = safeOneLine(valueFromKeys(values, "email"))
	}
	if name == "" {
		name = safeOneLine(valueFromKeys(values, "full_name", "name"))
	}

	if looksLikeEmail(name) && !looksLikeEmail(email) {
		name, email = email, name
	}
	if !looksLikeEmail(email) && looksLikeEmail(name) {
		name, email = email, name
	}
	if email != "" && !looksLikeEmail(email) {
		email = ""
	}
	return name, email
}

func valueFromKeys(values map[string]any, keys ...string) string {
	if values == nil {
		return ""
	}
	for _, k := range keys {
		for kk, v := range values {
			if strings.EqualFold(kk, k) {
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

func prettifyKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return "Response"
	}

	lk := strings.ToLower(k)
	switch lk {
	case "full_name":
		return "Full name"
	case "first_name":
		return "First name"
	case "last_name":
		return "Last name"
	case "email":
		return "Email"
	case "phone", "contact_number":
		return "Phone"
	case "address", "contact_address":
		return "Address"
	}

	if strings.HasPrefix(lk, "field_") {
		return "Field " + strings.TrimPrefix(lk, "field_")
	}

	parts := strings.FieldsFunc(k, func(r rune) bool { return r == '_' || r == '-' })
	for i := range parts {
		parts[i] = titleWord(parts[i])
	}
	return strings.Join(parts, " ")
}

func titleWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(strings.ToLower(s))
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func normalizeValueString(key string, v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return safeMultiLine(s)
	}
	if f, ok := v.(float64); ok {
		// Phone-like fields should never be exponent format
		lk := strings.ToLower(key)
		if strings.Contains(lk, "phone") || strings.Contains(lk, "field_4") {
			if f == math.Trunc(f) {
				return strconv.FormatInt(int64(f), 10)
			}
			return strconv.FormatFloat(f, 'f', 0, 64)
		}
		if f == math.Trunc(f) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	switch t := v.(type) {
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	}
	return safeMultiLine(fmt.Sprint(v))
}

func normalizePhone(raw string) string {
	s := safeOneLine(raw)
	if s == "" {
		return ""
	}

	// Handle "8.060974191e+09"
	ls := strings.ToLower(s)
	if strings.Contains(ls, "e+") || strings.Contains(ls, "e-") {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if f == math.Trunc(f) {
				s = strconv.FormatInt(int64(f), 10)
			} else {
				s = strconv.FormatFloat(f, 'f', 0, 64)
			}
		}
	}

	re := regexp.MustCompile(`[^\d+]`)
	s = re.ReplaceAllString(s, "")
	return s
}
