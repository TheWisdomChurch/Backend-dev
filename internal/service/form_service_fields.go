// internal/service/form_service_fields.go
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gorm.io/datatypes"

	"wisdomHouse-backend/internal/models"
)

/* =========================
   Field validation/build
========================= */

func buildAndValidateFields(formID string, fields []models.FormFieldDTO, draftOK bool) ([]models.FormField, error) {
	if !draftOK && len(fields) == 0 {
		return nil, errors.New("add at least one field")
	}

	type normalizedField struct {
		dto   models.FormFieldDTO
		key   string
		label string
		typ   string
	}

	seenKey := map[string]bool{}
	normalized := make([]normalizedField, 0, len(fields))
	out := make([]models.FormField, 0, len(fields))
	hasEmailCaptureField := false

	for i, f := range fields {
		label := strings.TrimSpace(f.Label)
		typ := normalizeFieldType(f.Type)
		key := strings.TrimSpace(f.Key)
		if key == "" && label != "" {
			key = slugify(label)
		}
		key = strings.ToLower(strings.TrimSpace(key))

		if key == "" {
			return nil, fmt.Errorf("field[%d]: key is required", i)
		}
		if label == "" {
			return nil, fmt.Errorf("field[%d]: label is required", i)
		}
		if seenKey[key] {
			return nil, fmt.Errorf("field[%d]: duplicate key '%s'", i, key)
		}
		seenKey[key] = true

		if !isValidFieldType(typ) {
			return nil, fmt.Errorf("field[%d]: invalid type '%s'", i, typ)
		}
		if typ == string(models.FieldEmail) ||
			strings.Contains(key, "email") ||
			strings.Contains(strings.ToLower(label), "email") {
			hasEmailCaptureField = true
		}

		normalized = append(normalized, normalizedField{
			dto:   f,
			key:   key,
			label: label,
			typ:   typ,
		})
	}

	for i, nf := range normalized {
		f := nf.dto

		var optionsJSON datatypes.JSON
		if nf.typ == string(models.FieldSelect) || nf.typ == string(models.FieldRadio) {
			if len(f.Options) == 0 {
				return nil, fmt.Errorf("field[%d]: options required for type '%s'", i, nf.typ)
			}
			seenVal := map[string]bool{}
			for oi, opt := range f.Options {
				if strings.TrimSpace(opt.Label) == "" || strings.TrimSpace(opt.Value) == "" {
					return nil, fmt.Errorf("field[%d].options[%d]: label and value required", i, oi)
				}
				if seenVal[opt.Value] {
					return nil, fmt.Errorf("field[%d].options[%d]: duplicate value '%s'", i, oi, opt.Value)
				}
				seenVal[opt.Value] = true
			}
			b, _ := json.Marshal(f.Options)
			optionsJSON = datatypes.JSON(b)
		} else if nf.typ == string(models.FieldCheckbox) && len(f.Options) > 0 {
			seenVal := map[string]bool{}
			for oi, opt := range f.Options {
				if strings.TrimSpace(opt.Label) == "" || strings.TrimSpace(opt.Value) == "" {
					return nil, fmt.Errorf("field[%d].options[%d]: label and value required", i, oi)
				}
				if seenVal[opt.Value] {
					return nil, fmt.Errorf("field[%d].options[%d]: duplicate value '%s'", i, oi, opt.Value)
				}
				seenVal[opt.Value] = true
			}
			b, _ := json.Marshal(f.Options)
			optionsJSON = datatypes.JSON(b)
		} else {
			optionsJSON = datatypes.JSON([]byte("null"))
		}

		validationJSON, err := normalizeValidationRules(nf.typ, f.Validation)
		if err != nil {
			return nil, fmt.Errorf("field[%d]: %w", i, err)
		}

		visibilityJSON, err := normalizeVisibility(f.Visibility, seenKey, nf.key)
		if err != nil {
			return nil, fmt.Errorf("field[%d]: %w", i, err)
		}

		out = append(out, models.FormField{
			FormID:     formID,
			Key:        nf.key,
			Label:      nf.label,
			Type:       models.FormFieldType(nf.typ),
			Required:   f.Required,
			Options:    optionsJSON,
			Validation: validationJSON,
			Visibility: visibilityJSON,
			Order:      f.Order,
		})
	}
	if !draftOK && !hasEmailCaptureField {
		return nil, errors.New("published forms must include an email field so confirmation emails can be delivered")
	}

	return out, nil
}

func isValidFieldType(t string) bool {
	switch t {
	case string(models.FieldText),
		string(models.FieldEmail),
		string(models.FieldTel),
		string(models.FieldTextarea),
		string(models.FieldSelect),
		string(models.FieldCheckbox),
		string(models.FieldRadio),
		string(models.FieldNumber),
		string(models.FieldDate),
		string(models.FieldImage):
		return true
	default:
		return false
	}
}

func normalizeFieldType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "phone":
		return string(models.FieldTel)
	case "dropdown", "select_one", "single_select", "selectone":
		return string(models.FieldSelect)
	case "paragraph", "long_text", "longtext", "multi_line", "multiline":
		return string(models.FieldTextarea)
	case "short_text", "shorttext", "single_line", "singleline":
		return string(models.FieldText)
	default:
		return t
	}
}

// normalizeValidationRules ensures provided validation config is coherent for the field type
// and returns a compact JSON blob (or null) to store.
func normalizeValidationRules(fieldType string, v *models.FormFieldValidation) (datatypes.JSON, error) {
	if v == nil {
		return datatypes.JSON([]byte("null")), nil
	}

	rules := *v // shallow copy so we can sanitize

	// Basic numeric checks
	if rules.MinLength != nil && *rules.MinLength < 0 {
		return nil, fmt.Errorf("minLength cannot be negative")
	}
	if rules.MaxLength != nil && *rules.MaxLength < 0 {
		return nil, fmt.Errorf("maxLength cannot be negative")
	}
	if rules.MinLength != nil && rules.MaxLength != nil && *rules.MaxLength < *rules.MinLength {
		return nil, fmt.Errorf("maxLength cannot be less than minLength")
	}
	if rules.MaxWords != nil && *rules.MaxWords <= 0 {
		return nil, fmt.Errorf("maxWords must be greater than 0")
	}
	if rules.Min != nil && rules.Max != nil && *rules.Max < *rules.Min {
		return nil, fmt.Errorf("max cannot be less than min")
	}

	// Allow numeric bounds only on number fields
	if fieldType != string(models.FieldNumber) && (rules.Min != nil || rules.Max != nil) {
		return nil, fmt.Errorf("min/max are only valid for number fields")
	}

	// Allow string-oriented rules only on string-input fields
	if !isStringFieldType(fieldType) {
		if rules.MinLength != nil || rules.MaxLength != nil || rules.MaxWords != nil || rules.Pattern != nil {
			return nil, fmt.Errorf("string validation rules are not applicable to this field type")
		}
	}

	if rules.Pattern != nil {
		p := strings.TrimSpace(*rules.Pattern)
		if p == "" {
			rules.Pattern = nil
		} else {
			if _, err := regexp.Compile(p); err != nil {
				return nil, fmt.Errorf("pattern is not a valid regex")
			}
			rules.Pattern = &p
		}
	}

	if !hasAnyValidationRule(&rules) {
		return datatypes.JSON([]byte("null")), nil
	}

	b, err := json.Marshal(rules)
	if err != nil {
		return nil, errors.New("invalid validation rules")
	}
	return datatypes.JSON(b), nil
}

func normalizeVisibility(v *models.FormFieldVisibility, keys map[string]bool, selfKey string) (datatypes.JSON, error) {
	if v == nil || len(v.Rules) == 0 {
		return datatypes.JSON([]byte("null")), nil
	}

	match := strings.ToLower(strings.TrimSpace(v.Match))
	if match == "" {
		match = "all"
	}
	if match != "all" && match != "any" {
		return nil, fmt.Errorf("visibility.match must be 'all' or 'any'")
	}
	v.Match = match

	for i := range v.Rules {
		r := v.Rules[i]
		key := strings.ToLower(strings.TrimSpace(r.FieldKey))
		if key == "" {
			return nil, fmt.Errorf("visibility.rules[%d].fieldKey is required", i)
		}
		if key == selfKey {
			return nil, fmt.Errorf("visibility.rules[%d].fieldKey cannot reference itself", i)
		}
		if keys != nil && !keys[key] {
			return nil, fmt.Errorf("visibility.rules[%d].fieldKey '%s' not found", i, key)
		}

		op := strings.ToLower(strings.TrimSpace(r.Operator))
		if op == "" {
			return nil, fmt.Errorf("visibility.rules[%d].operator is required", i)
		}
		switch op {
		case "equals", "not_equals":
			if r.Value == nil {
				return nil, fmt.Errorf("visibility.rules[%d].value is required", i)
			}
		case "in", "not_in":
			if len(r.Values) == 0 {
				return nil, fmt.Errorf("visibility.rules[%d].values is required", i)
			}
		default:
			return nil, fmt.Errorf("visibility.rules[%d].operator must be one of: equals, not_equals, in, not_in", i)
		}

		r.FieldKey = key
		r.Operator = op
		v.Rules[i] = r
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, errors.New("invalid visibility rules")
	}
	return datatypes.JSON(b), nil
}

func isStringFieldType(t string) bool {
	switch t {
	case string(models.FieldText),
		string(models.FieldEmail),
		string(models.FieldTel),
		string(models.FieldTextarea),
		string(models.FieldSelect),
		string(models.FieldRadio),
		string(models.FieldDate),
		string(models.FieldImage):
		return true
	default:
		return false
	}
}

func hasAnyValidationRule(v *models.FormFieldValidation) bool {
	return v != nil &&
		(v.MinLength != nil ||
			v.MaxLength != nil ||
			v.MaxWords != nil ||
			v.Pattern != nil ||
			v.Min != nil ||
			v.Max != nil)
}

func decodeVisibility(j datatypes.JSON) *models.FormFieldVisibility {
	if len(j) == 0 || string(j) == "null" {
		return nil
	}
	var v models.FormFieldVisibility
	if err := json.Unmarshal(j, &v); err != nil {
		return nil
	}
	if len(v.Rules) == 0 {
		return nil
	}
	if strings.TrimSpace(v.Match) == "" {
		v.Match = "all"
	}
	return &v
}

func normalizeVisibilityToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isAffirmativeVisibilityOption(value string) bool {
	switch normalizeVisibilityToken(value) {
	case "yes", "true", "1":
		return true
	default:
		return false
	}
}

func isNegativeVisibilityOption(value string) bool {
	switch normalizeVisibilityToken(value) {
	case "no", "false", "0":
		return true
	default:
		return false
	}
}

func looksLikeImplicitYesField(label string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), "if yes")
}

func findImplicitYesOptionValue(field models.FormField) (string, bool) {
	if field.Type != models.FieldRadio && field.Type != models.FieldSelect {
		return "", false
	}

	opts := decodeOptionsToDTO(field.Options)
	if len(opts) == 0 {
		return "", false
	}

	yesValue := ""
	hasNoOption := false
	for _, opt := range opts {
		label := strings.TrimSpace(opt.Label)
		value := strings.TrimSpace(opt.Value)

		if yesValue == "" && (isAffirmativeVisibilityOption(label) || isAffirmativeVisibilityOption(value)) {
			if value != "" {
				yesValue = value
			} else {
				yesValue = label
			}
		}
		if isNegativeVisibilityOption(label) || isNegativeVisibilityOption(value) {
			hasNoOption = true
		}
	}

	if yesValue == "" || !hasNoOption {
		return "", false
	}
	return yesValue, true
}

func applyImplicitFieldVisibilityDefaults(fields []models.FormField) []models.FormField {
	if len(fields) == 0 {
		return fields
	}

	enriched := append([]models.FormField(nil), fields...)
	sort.SliceStable(enriched, func(i, j int) bool {
		if enriched[i].Order == enriched[j].Order {
			return i < j
		}
		return enriched[i].Order < enriched[j].Order
	})

	for i := range enriched {
		if decodeVisibility(enriched[i].Visibility) != nil {
			continue
		}
		if !looksLikeImplicitYesField(enriched[i].Label) {
			continue
		}

		for prev := i - 1; prev >= 0; prev-- {
			fieldKey := strings.TrimSpace(enriched[prev].Key)
			yesValue, ok := findImplicitYesOptionValue(enriched[prev])
			if !ok || fieldKey == "" {
				continue
			}

			vis := models.FormFieldVisibility{
				Match: "all",
				Rules: []models.FormFieldCondition{
					{
						FieldKey: fieldKey,
						Operator: "equals",
						Value:    yesValue,
					},
				},
			}
			if raw, err := json.Marshal(vis); err == nil {
				enriched[i].Visibility = datatypes.JSON(raw)
			}
			break
		}
	}

	return enriched
}

func isFieldVisible(f models.FormField, values map[string]any) bool {
	vis := decodeVisibility(f.Visibility)
	if vis == nil || len(vis.Rules) == 0 {
		return true
	}

	match := strings.ToLower(strings.TrimSpace(vis.Match))
	if match != "any" && match != "all" {
		match = "all"
	}

	if match == "any" {
		for _, r := range vis.Rules {
			if evaluateVisibilityRule(r, values) {
				return true
			}
		}
		return false
	}

	for _, r := range vis.Rules {
		if !evaluateVisibilityRule(r, values) {
			return false
		}
	}
	return true
}

func evaluateVisibilityRule(r models.FormFieldCondition, values map[string]any) bool {
	key := strings.TrimSpace(r.FieldKey)
	if key == "" {
		return false
	}
	val, ok := values[key]
	if !ok || val == nil {
		return false
	}

	op := strings.ToLower(strings.TrimSpace(r.Operator))
	switch op {
	case "equals":
		return valueEquals(val, r.Value)
	case "not_equals":
		return !valueEquals(val, r.Value)
	case "in":
		return valueIn(val, r.Values)
	case "not_in":
		return !valueIn(val, r.Values)
	default:
		return false
	}
}

func valueEquals(a any, b any) bool {
	if a == nil || b == nil {
		return false
	}

	if list, ok := toAnySlice(a); ok {
		for _, item := range list {
			if valueEquals(item, b) {
				return true
			}
		}
		return false
	}

	if list, ok := toAnySlice(b); ok {
		for _, item := range list {
			if valueEquals(a, item) {
				return true
			}
		}
		return false
	}

	if av, ok := toBoolValue(a); ok {
		if bv, ok := toBoolValue(b); ok {
			return av == bv
		}
	}

	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			return strings.EqualFold(strings.TrimSpace(as), strings.TrimSpace(bs))
		}
	}

	if af, err := toFloat64(a); err == nil {
		if bf, err := toFloat64(b); err == nil {
			return af == bf
		}
	}

	return fmt.Sprint(a) == fmt.Sprint(b)
}

func valueIn(val any, list []any) bool {
	if len(list) == 0 {
		return false
	}

	if values, ok := toAnySlice(val); ok {
		for _, item := range values {
			if valueIn(item, list) {
				return true
			}
		}
		return false
	}

	for _, v := range list {
		if valueEquals(val, v) {
			return true
		}
	}
	return false
}

func toAnySlice(value any) ([]any, bool) {
	switch list := value.(type) {
	case []any:
		return list, true
	case []string:
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = item
		}
		return out, true
	case []bool:
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = item
		}
		return out, true
	case []float64:
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = item
		}
		return out, true
	case []int:
		out := make([]any, len(list))
		for i, item := range list {
			out[i] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func toBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(typed)))
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
}
