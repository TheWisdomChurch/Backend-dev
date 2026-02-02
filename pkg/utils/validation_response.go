package utils

import "github.com/gin-gonic/gin"

// FieldError represents a single field-level validation error.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// ValidationErrorResponse sends a structured validation error response.
func ValidationErrorResponse(c *gin.Context, message string, errs []FieldError) {
	if message == "" {
		message = "Validation failed"
	}
	c.JSON(400, gin.H{
		"status":  "error",
		"message": message,
		"errors":  errs,
	})
}
