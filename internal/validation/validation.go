package validation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"wisdomHouse-backend/pkg/utils"
)

// Init configures validation behavior and registers custom tags.
func Init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			if name == "" {
				return fld.Name
			}
			return name
		})

		_ = v.RegisterValidation("rfc3339", validateRFC3339)
		_ = v.RegisterValidation("date_ymd", validateDateYMD)
		_ = v.RegisterValidation("phone", validatePhone)
	}
}

// BindJSON binds and validates JSON payloads with consistent error responses.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		writeValidationError(c, err)
		return false
	}
	return true
}

// BindQuery binds and validates query params with consistent error responses.
func BindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		writeValidationError(c, err)
		return false
	}
	return true
}

func writeValidationError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	if errs := collectFieldErrors(err); len(errs) > 0 {
		utils.ValidationErrorResponse(c, "Validation failed", errs)
		return
	}

	// Fallback for malformed JSON or other binding errors
	utils.ValidationErrorResponse(c, "Invalid request payload", []utils.FieldError{
		{
			Field:   "",
			Code:    "invalid_payload",
			Message: err.Error(),
		},
	})
}

func collectFieldErrors(err error) []utils.FieldError {
	if err == nil {
		return nil
	}

	// JSON decoding errors
	switch e := err.(type) {
	case *json.SyntaxError:
		return []utils.FieldError{{
			Field:   "",
			Code:    "json_syntax",
			Message: "Invalid JSON payload",
		}}
	case *json.UnmarshalTypeError:
		field := strings.TrimSpace(e.Field)
		msgField := field
		if msgField == "" {
			msgField = "value"
		}
		return []utils.FieldError{{
			Field:   field,
			Code:    "type",
			Message: fmt.Sprintf("%s must be %s", msgField, e.Type.String()),
		}}
	}

	// Validation errors
	ve, ok := err.(validator.ValidationErrors)
	if !ok {
		return nil
	}

	out := make([]utils.FieldError, 0, len(ve))
	for _, fe := range ve {
		field := strings.TrimSpace(fe.Field())
		if field == "" {
			field = fe.StructField()
		}
		code := fe.Tag()
		out = append(out, utils.FieldError{
			Field:   field,
			Code:    code,
			Message: validationMessage(field, fe),
		})
	}
	return out
}

func validationMessage(field string, fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email", field)
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", field, fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of [%s]", field, fe.Param())
	case "rfc3339":
		return fmt.Sprintf("%s must be RFC3339 or YYYY-MM-DDTHH:MM", field)
	case "date_ymd":
		return fmt.Sprintf("%s must be a valid date (YYYY-MM-DD)", field)
	case "phone":
		return fmt.Sprintf("%s must be a valid phone number", field)
	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}

func validateRFC3339(fl validator.FieldLevel) bool {
	val := strings.TrimSpace(fl.Field().String())
	if val == "" {
		return true
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, val); err == nil {
			return true
		}
	}
	return false
}

func validateDateYMD(fl validator.FieldLevel) bool {
	val := strings.TrimSpace(fl.Field().String())
	if val == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", val)
	return err == nil
}

func validatePhone(fl validator.FieldLevel) bool {
	val := strings.TrimSpace(fl.Field().String())
	if val == "" {
		return true
	}
	// Basic permissive phone validation (digits, spaces, +, -, parentheses)
	for _, r := range val {
		if (r >= '0' && r <= '9') || r == '+' || r == '-' || r == ' ' || r == '(' || r == ')' {
			continue
		}
		return false
	}
	return len(val) >= 7 && len(val) <= 20
}
