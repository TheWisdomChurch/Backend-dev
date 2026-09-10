// internal/service/form_service_validation.go
package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
)

/* =========================
   Date regexes (self-contained)
   — delete these if identical names already exist elsewhere
========================= */

var (
	// D-M or DD-MM (also accepts / and . as separators)
	dateDayMonthRe = regexp.MustCompile(`^(\d{1,2})[-/.](\d{1,2})$`)

	// D-M-YYYY or DD-MM-YYYY (also accepts / and . ; 2- or 4-digit years)
	dateFullRe = regexp.MustCompile(`^(\d{1,2})[-/.](\d{1,2})[-/.](\d{2}|\d{4})$`)

	// ISO date or datetime: YYYY-MM-DD or YYYY-MM-DDTHH:MM[:SS[.fff]][Z|±HH:MM]
	dateISORe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})(?:[T ]\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?)?$`)
)

/* =========================
   Submission validation
========================= */

func validateSubmission(fields []models.FormField, values map[string]any) (map[string]any, error) {
	fields = applyImplicitFieldVisibilityDefaults(fields)

	fieldByKey := map[string]models.FormField{}
	for _, f := range fields {
		fieldByKey[f.Key] = f
	}

	for k := range values {
		if _, ok := fieldByKey[k]; !ok {
			return nil, fmt.Errorf("unknown field '%s'", k)
		}
	}

	// snapshot values for visibility evaluation
	snapshot := make(map[string]any, len(values))
	for k, v := range values {
		snapshot[k] = v
	}

	clean := make(map[string]any, len(values))

	for _, f := range fields {
		v, exists := snapshot[f.Key]

		if !isFieldVisible(f, snapshot) {
			continue
		}

		rules := decodeValidation(f.Validation)

		if !exists || v == nil {
			if f.Required {
				return nil, fmt.Errorf("field '%s' is required", f.Key)
			}
			continue
		}

		switch f.Type {
		case models.FieldCheckbox:
			opts := decodeOptionsToDTO(f.Options)
			if len(opts) > 0 {
				list, ok := toStringSlice(v)
				if !ok {
					return nil, fmt.Errorf("field '%s' must be a list of strings", f.Key)
				}

				allowed := map[string]bool{}
				for _, o := range opts {
					allowed[o.Value] = true
				}

				seen := map[string]bool{}
				cleaned := make([]string, 0, len(list))
				for _, raw := range list {
					s := strings.TrimSpace(raw)
					if s == "" {
						continue
					}
					if !allowed[s] {
						return nil, fmt.Errorf("field '%s' has invalid option", f.Key)
					}
					if !seen[s] {
						seen[s] = true
						cleaned = append(cleaned, s)
					}
				}

				if f.Required && len(cleaned) == 0 {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}

				clean[f.Key] = cleaned
				continue
			}

			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be boolean", f.Key)
			}
			if f.Required && !b {
				return nil, fmt.Errorf("field '%s' must be accepted", f.Key)
			}
			clean[f.Key] = b

		case models.FieldNumber:
			if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
				if f.Required {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}
				continue
			}

			num, err := toFloat64(v)
			if err != nil {
				return nil, fmt.Errorf("field '%s' must be a number", f.Key)
			}
			clean[f.Key] = num

			if rules != nil {
				if rules.Min != nil && num < *rules.Min {
					return nil, fmt.Errorf("field '%s' must be >= %g", f.Key, *rules.Min)
				}
				if rules.Max != nil && num > *rules.Max {
					return nil, fmt.Errorf("field '%s' must be <= %g", f.Key, *rules.Max)
				}
			}

		case models.FieldSelect, models.FieldRadio:
			sv, ok := valueToString(v)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be string", f.Key)
			}
			sv = strings.TrimSpace(sv)
			if sv == "" {
				if f.Required {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}
				continue
			}

			opts := decodeOptionsToDTO(f.Options)
			allowed := map[string]bool{}
			for _, o := range opts {
				allowed[o.Value] = true
			}
			if !allowed[sv] {
				return nil, fmt.Errorf("field '%s' has invalid option", f.Key)
			}
			clean[f.Key] = sv

			if err := applyStringRules(f.Key, sv, rules); err != nil {
				return nil, err
			}

		case models.FieldImage:
			sv, ok := valueToString(v)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be image content", f.Key)
			}
			sv = strings.TrimSpace(sv)
			if sv == "" {
				if f.Required {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}
				continue
			}
			if err := validateImageFieldValue(sv); err != nil {
				return nil, fmt.Errorf("field '%s' %s", f.Key, err.Error())
			}
			clean[f.Key] = sv

		default: // text, textarea, email, tel, date
			sv, ok := valueToString(v)
			if !ok {
				return nil, fmt.Errorf("field '%s' must be string", f.Key)
			}
			sv = strings.TrimSpace(sv)
			if sv == "" {
				if f.Required {
					return nil, fmt.Errorf("field '%s' is required", f.Key)
				}
				continue
			}

			switch f.Type {
			case models.FieldEmail:
				if !emailRe.MatchString(sv) {
					return nil, fmt.Errorf("field '%s' must be a valid email", f.Key)
				}
			case models.FieldTel:
				if !phoneRe.MatchString(sv) {
					return nil, fmt.Errorf("field '%s' must be a valid phone number", f.Key)
				}
			case models.FieldDate:
				fullYear := dateFieldKeepsYear(rules, f.Key, f.Label)
				normalizedDate, err := normalizePublicFormDateValue(sv, fullYear)
				if err != nil && fullYear {
					// A full-date field that was sent day+month only (e.g. an
					// older client, or the field's mode changed after the form
					// went live) — accept it rather than 400, storing what we
					// got. The renderer now always captures the year, so this
					// is only a migration cushion.
					if dm, dmErr := normalizePublicFormDateValue(sv, false); dmErr == nil {
						normalizedDate, err = dm, nil
					}
				}
				if err != nil {
					expected := "DD-MM"
					if fullYear {
						expected = "DD-MM-YYYY"
					}
					return nil, fmt.Errorf(
						"field '%s' must be a valid date (%s): %s",
						f.Key, expected, err.Error(),
					)
				}
				sv = normalizedDate
				// Date values are canonicalised above; free-text rules
				// (pattern / min / max length) never apply to them.
				clean[f.Key] = sv
				continue
			}

			if err := applyStringRules(f.Key, sv, rules); err != nil {
				return nil, err
			}
			clean[f.Key] = sv
		}
	}

	return clean, nil
}

func applyStringRules(key, value string, rules *models.FormFieldValidation) error {
	if rules == nil {
		return nil
	}

	runeLen := utf8.RuneCountInString(value)
	if rules.MinLength != nil && runeLen < *rules.MinLength {
		return fmt.Errorf("field '%s' must be at least %d characters", key, *rules.MinLength)
	}
	if rules.MaxLength != nil && runeLen > *rules.MaxLength {
		return fmt.Errorf("field '%s' must be at most %d characters", key, *rules.MaxLength)
	}
	if rules.MaxWords != nil {
		if wc := countWords(value); wc > *rules.MaxWords {
			return fmt.Errorf("field '%s' must be at most %d words", key, *rules.MaxWords)
		}
	}
	if rules.Pattern != nil && strings.TrimSpace(*rules.Pattern) != "" {
		// A malformed stored pattern must not panic the submission handler.
		if re, err := regexp.Compile(*rules.Pattern); err == nil && !re.MatchString(value) {
			return fmt.Errorf("field '%s' does not match the required format", key)
		}
	}
	return nil
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

// birthDateFieldRe matches a field key/label that clearly means a date of birth
// and therefore needs the year kept. Plain "birthday" is deliberately excluded —
// that feeds recurring-greeting automation which only wants day+month.
var birthDateFieldRe = regexp.MustCompile(`(?i)\b(d\.?o\.?b|date[\s_-]*of[\s_-]*birth|birth[\s_-]*date)\b`)

// dateFieldKeepsYear decides whether a `date` field stores DD-MM-YYYY: the
// admin's explicit dateMode wins, otherwise an obvious date-of-birth field
// keeps the year automatically.
func dateFieldKeepsYear(rules *models.FormFieldValidation, key, label string) bool {
	if rules != nil && rules.DateMode != nil {
		switch strings.ToLower(strings.TrimSpace(*rules.DateMode)) {
		case "full":
			return true
		case "day-month":
			return false
		}
	}
	return birthDateFieldRe.MatchString(key + " " + label)
}

// normalizePublicFormDateValue canonicalises a submitted date.
//
//   - fullYear == false → returns "DD-MM" (day + month only; used for recurring
//     birthday/anniversary automation).
//   - fullYear == true  → returns "DD-MM-YYYY"; a realistic year is required.
//
// Accepted input formats (all zero-padding is applied on output):
//
//	"D-M", "DD-MM"                                -> "DD-MM"
//	"D-M-YY", "D-M-YYYY", "DD-MM-YYYY"            -> "DD-MM" or "DD-MM-YYYY"
//	"D/M", "D/M/YYYY", "D.M.YYYY"                 -> same
//	"YYYY-MM-DD", "YYYY-MM-DDTHH:MM:SS(.sss)Z"    -> same
//
// The returned error always includes the offending value so the API surface
// stays useful when a client sends something we can't parse.
func normalizePublicFormDateValue(value string, fullYear bool) (string, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return "", errors.New("date is empty")
	}

	var day, month, year int
	var hasYear bool

	switch {
	case dateFullRe.MatchString(val):
		m := dateFullRe.FindStringSubmatch(val)
		day, _ = strconv.Atoi(m[1])
		month, _ = strconv.Atoi(m[2])
		year, _ = strconv.Atoi(m[3])
		hasYear = true

	case dateDayMonthRe.MatchString(val):
		m := dateDayMonthRe.FindStringSubmatch(val)
		day, _ = strconv.Atoi(m[1])
		month, _ = strconv.Atoi(m[2])

	case dateISORe.MatchString(val):
		m := dateISORe.FindStringSubmatch(val)
		year, _ = strconv.Atoi(m[1])
		month, _ = strconv.Atoi(m[2])
		day, _ = strconv.Atoi(m[3])
		hasYear = true

	default:
		return "", fmt.Errorf("unrecognised date format %q", val)
	}

	if month < 1 || month > 12 {
		return "", fmt.Errorf("month out of range in %q", val)
	}
	if day < 1 || day > daysInMonth(month, year) {
		return "", fmt.Errorf("day out of range in %q", val)
	}

	// Expand a two-digit year (e.g. "05" -> 2005, "85" -> 1985).
	if hasYear && year >= 0 && year < 100 {
		if year <= 30 {
			year += 2000
		} else {
			year += 1900
		}
	}

	if fullYear {
		if !hasYear {
			return "", fmt.Errorf("year required in %q for a full date", val)
		}
		maxYear := time.Now().Year() + 1
		if year < 1900 || year > maxYear {
			return "", fmt.Errorf("year %d out of range in %q", year, val)
		}
		if err := validCalendarDate(day, month, year); err != nil {
			return "", fmt.Errorf("%s in %q", err.Error(), val)
		}
		return fmt.Sprintf("%02d-%02d-%04d", day, month, year), nil
	}

	// Day+month mode: ignore any year that was supplied (frontends often send
	// the current year for "birthday"-style fields).
	return fmt.Sprintf("%02d-%02d", day, month), nil
}

// daysInMonth returns the maximum valid day for the given month. When year is
// 0 it allows 29 for February so leap-day birthdays are accepted in day-month
// mode. When a real year is supplied it applies the Gregorian leap rule.
func daysInMonth(month, year int) int {
	switch month {
	case 2:
		if year == 0 || isLeapYear(year) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	default:
		return 31
	}
}

func isLeapYear(y int) bool {
	return (y%4 == 0 && y%100 != 0) || y%400 == 0
}

func validCalendarDate(day, month, year int) error {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day || int(t.Month()) != month || t.Year() != year {
		return errors.New("invalid calendar date")
	}
	return nil
}

func validateImageFieldValue(value string) error {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, err := url.Parse(value)
		if err != nil || strings.TrimSpace(u.Host) == "" {
			return errors.New("must contain a valid image URL")
		}
		return nil
	}

	if !dataImageRe.MatchString(lower) {
		return errors.New("must be a valid image (JPEG, PNG, or WebP)")
	}

	comma := strings.Index(value, ",")
	if comma < 0 || comma+1 >= len(value) {
		return errors.New("contains invalid image data")
	}

	decoded, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil {
		return errors.New("contains invalid image data")
	}

	const maxBytes = 8 * 1024 * 1024
	if len(decoded) > maxBytes {
		return errors.New("must be 8MB or smaller")
	}

	return nil
}

func firstToken(s string) string {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func valueToString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func toStringSlice(v any) ([]string, bool) {
	switch raw := v.(type) {
	case []string:
		return raw, true
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	case string:
		if strings.TrimSpace(raw) == "" {
			return []string{}, true
		}
		return []string{raw}, true
	default:
		return nil, false
	}
}

func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case json.Number:
		return n.Float64()
	case string:
		s := strings.TrimSpace(n)
		return strconv.ParseFloat(s, 64)
	default:
		return 0, fmt.Errorf("not a number")
	}
}

func decodeValidation(j datatypes.JSON) *models.FormFieldValidation {
	if len(j) == 0 || string(j) == "null" {
		return nil
	}
	var v models.FormFieldValidation
	if err := json.Unmarshal(j, &v); err != nil {
		return nil
	}
	if !hasAnyValidationRule(&v) {
		return nil
	}
	return &v
}

// extractCommonFields pulls common contact fields from dynamic values map for analytics.
// It prefers known keys, then falls back to matching field types (email/tel) if present.
func extractCommonFields(fields []models.FormField, values map[string]any) (*string, *string, *string, *string) {
	lookup := func(keys ...string) *string {
		for _, k := range keys {
			if v, ok := values[k]; ok {
				if s, ok := v.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						return &s
					}
				}
			}
		}
		return nil
	}

	byType := func(t models.FormFieldType) *string {
		for _, f := range fields {
			if f.Type != t {
				continue
			}
			if v, ok := values[f.Key]; ok {
				if s, ok := v.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						return &s
					}
				}
			}
		}
		return nil
	}

	lookupByLabel := func(needles ...string) *string {
		for _, f := range fields {
			label := strings.ToLower(strings.TrimSpace(f.Label))
			if label == "" {
				continue
			}
			matched := false
			for _, needle := range needles {
				n := strings.ToLower(strings.TrimSpace(needle))
				if n == "" {
					continue
				}
				if strings.Contains(label, n) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			if raw, ok := values[f.Key]; ok {
				if s, ok := raw.(string); ok {
					v := strings.TrimSpace(s)
					if v != "" {
						return &v
					}
				}
			}
		}
		return nil
	}

	normalizeName := func(primary *string) *string {
		if primary != nil {
			v := strings.TrimSpace(*primary)
			if v != "" && !emailRe.MatchString(v) {
				return &v
			}
		}

		first := lookup("firstName", "first_name", "firstname", "first")
		last := lookup("lastName", "last_name", "lastname", "last")
		if first == nil {
			first = lookupByLabel("first name", "firstname")
		}
		if last == nil {
			last = lookupByLabel("last name", "lastname", "surname")
		}

		switch {
		case first != nil && last != nil:
			combined := strings.TrimSpace(*first + " " + *last)
			if combined != "" {
				return &combined
			}
		case first != nil:
			v := strings.TrimSpace(*first)
			if v != "" {
				return &v
			}
		case last != nil:
			v := strings.TrimSpace(*last)
			if v != "" {
				return &v
			}
		}

		legacy := lookup("email")
		if legacy != nil {
			v := strings.TrimSpace(*legacy)
			if v != "" && !emailRe.MatchString(v) {
				return &v
			}
		}

		return nil
	}

	normalizeEmail := func(candidate *string) *string {
		if candidate != nil {
			v := strings.TrimSpace(*candidate)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		if typed := byType(models.FieldEmail); typed != nil {
			v := strings.TrimSpace(*typed)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		if labelled := lookupByLabel("email", "e-mail"); labelled != nil {
			v := strings.TrimSpace(*labelled)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		for _, raw := range values {
			s, ok := raw.(string)
			if !ok {
				continue
			}
			v := strings.TrimSpace(s)
			if emailRe.MatchString(v) {
				return &v
			}
		}

		return nil
	}

	name := normalizeName(lookup("fullName", "name", "full_name"))
	email := normalizeEmail(lookup("email", "contactEmail"))
	phone := lookup("phone", "contactPhone", "contactNumber", "phoneNumber")
	if phone == nil {
		phone = byType(models.FieldTel)
	}
	addr := lookup("address", "contactAddress")

	return name, email, phone, addr
}
