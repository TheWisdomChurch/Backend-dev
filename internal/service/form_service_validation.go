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
				normalizedDate, err := normalizePublicFormDateValue(sv)
				if err != nil {
					return nil, fmt.Errorf("field '%s' must be a valid date (DD-MM)", f.Key)
				}
				sv = normalizedDate
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
	if rules.Pattern != nil {
		re := regexp.MustCompile(*rules.Pattern)
		if !re.MatchString(value) {
			return fmt.Errorf("field '%s' does not match the required format", key)
		}
	}
	return nil
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

func normalizePublicFormDateValue(value string) (string, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return "", errors.New("date is empty")
	}

	// Preferred user-facing format: DD-MM or DD-MM-YYYY.
	if m := ddDashRe.FindStringSubmatch(val); len(m) >= 3 {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		if month < 1 || month > 12 {
			return "", errors.New("month out of range")
		}
		maxDay := daysInMonth(month)
		if day < 1 || day > maxDay {
			return "", errors.New("day out of range")
		}
		return fmt.Sprintf("%02d-%02d", day, month), nil
	}

	// Backward-compatible: DD/MM or DD/MM/YYYY.
	if m := ddSlashRe.FindStringSubmatch(val); len(m) >= 3 {
		day, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		if month < 1 || month > 12 {
			return "", errors.New("month out of range")
		}
		maxDay := daysInMonth(month)
		if day < 1 || day > maxDay {
			return "", errors.New("day out of range")
		}
		return fmt.Sprintf("%02d-%02d", day, month), nil
	}

	// Backward-compatible: YYYY-MM-DD (legacy clients).
	if t, err := time.Parse("2006-01-02", val); err == nil {
		return fmt.Sprintf("%02d-%02d", t.Day(), int(t.Month())), nil
	}

	return "", errors.New("invalid date format")
}

func daysInMonth(month int) int {
	switch month {
	case 2:
		return 29
	case 4, 6, 9, 11:
		return 30
	default:
		return 31
	}
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
		// allow single selection serialized as string
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

		// Legacy fallback: some older forms used "email" key for first-name text input.
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
