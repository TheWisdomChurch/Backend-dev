// Enterprise-grade error handler for all API responses
package apperror

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ErrorResponse standardized API error response
type ErrorResponse struct {
	Status    string                 `json:"status"`
	Message   string                 `json:"message"`
	Code      string                 `json:"code,omitempty"`
	Errors    map[string]string      `json:"errors,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// ErrorCode standardized error codes
type ErrorCode string

const (
	// Client errors (4xx)
	CodeBadRequest       ErrorCode = "bad_request"
	CodeUnauthorized     ErrorCode = "unauthorized"
	CodeForbidden        ErrorCode = "forbidden"
	CodeNotFound         ErrorCode = "not_found"
	CodeConflict         ErrorCode = "conflict"
	CodeTooManyRequests  ErrorCode = "too_many_requests"
	CodeValidationError  ErrorCode = "validation_error"
	CodeDatabaseError    ErrorCode = "database_error"

	// Server errors (5xx)
	CodeInternalError    ErrorCode = "internal_error"
	CodeServiceUnavailable ErrorCode = "service_unavailable"
)

// AppError structured application error
type AppError struct {
	HTTPStatus  int
	Code        ErrorCode
	Message     string
	Details     map[string]string
	InternalErr error
	Timestamp   time.Time
}

// NewAppError creates a new app error
func NewAppError(status int, code ErrorCode, message string) *AppError {
	return &AppError{
		HTTPStatus: status,
		Code:       code,
		Message:    message,
		Timestamp:  time.Now(),
		Details:    make(map[string]string),
	}
}

// WithDetails adds error details
func (e *AppError) WithDetails(details map[string]string) *AppError {
	e.Details = details
	return e
}

// WithInternalError adds internal error for logging
func (e *AppError) WithInternalError(err error) *AppError {
	e.InternalErr = err
	return e
}

// GlobalErrorHandler processes all errors in a unified way
func GlobalErrorHandler(c *gin.Context) {
	c.Next()

	// Check if there was an error
	if len(c.Errors) == 0 {
		return
	}

	// Get last error
	ginErr := c.Errors.Last()
	var appErr *AppError

	// Try to unwrap AppError
	if errors.As(ginErr.Err, &appErr) {
		respondWithError(c, appErr)
		return
	}

	// Handle database errors
	if isDatabaseError(ginErr.Err) {
		respondWithError(c, NewAppError(
			http.StatusInternalServerError,
			CodeDatabaseError,
			"Database error occurred",
		).WithInternalError(ginErr.Err))
		return
	}

	// Handle validation errors
	if isValidationError(ginErr.Err) {
		respondWithError(c, NewAppError(
			http.StatusBadRequest,
			CodeValidationError,
			"Validation failed",
		).WithInternalError(ginErr.Err))
		return
	}

	// Generic error handler
	respondWithError(c, NewAppError(
		http.StatusInternalServerError,
		CodeInternalError,
		"An unexpected error occurred",
	).WithInternalError(ginErr.Err))
}

// respondWithError sends error response to client
func respondWithError(c *gin.Context, err *AppError) {
	// Log error with context
	logError(c, err)

	// Get request ID from context
	requestID := c.GetString("request_id")
	if requestID == "" {
		requestID = c.GetHeader("X-Request-ID")
	}

	// Build response
	response := ErrorResponse{
		Status:     "error",
		Message:    err.Message,
		Code:       string(err.Code),
		RequestID:  requestID,
		Timestamp:  err.Timestamp.Format(time.RFC3339),
		Errors:     err.Details,
	}

	// Add helpful metadata for specific errors
	response.Meta = getErrorMetadata(err)

	// Send response
	c.JSON(err.HTTPStatus, response)
}

// getErrorMetadata returns helpful metadata for specific error types
func getErrorMetadata(err *AppError) map[string]interface{} {
	meta := make(map[string]interface{})

	switch err.Code {
	case CodeTooManyRequests:
		meta["retry_after"] = 1
		meta["help"] = "You're making requests too quickly. Please wait a moment."

	case CodeDatabaseError:
		meta["help"] = "A database error occurred. Please contact support if this persists."
		meta["support_contact"] = "support@wisdomchurchhq.org"

	case CodeUnauthorized:
		meta["help"] = "Your session has expired. Please log in again."
		meta["action"] = "redirect_to_login"

	case CodeForbidden:
		meta["help"] = "You don't have permission to access this resource."

	case CodeValidationError:
		meta["help"] = "Please check your input and try again."

	}

	return meta
}

// logError logs error with full context
func logError(c *gin.Context, err *AppError) {
	requestID := c.GetString("request_id")
	userID := c.GetString("user_id")

	logEntry := fmt.Sprintf(
		"[%s] %s %s - %s (User: %s, Code: %s)",
		requestID,
		c.Request.Method,
		c.Request.URL.Path,
		err.Message,
		userID,
		err.Code,
	)

	// Log internal error if present
	if err.InternalErr != nil {
		logEntry += fmt.Sprintf(" - Internal: %v", err.InternalErr)
	}

	// Log based on status code
	switch {
	case err.HTTPStatus >= 500:
		log.Printf("ERROR: %s", logEntry)
	case err.HTTPStatus >= 400:
		log.Printf("WARNING: %s", logEntry)
	default:
		log.Printf("INFO: %s", logEntry)
	}
}

// Helper functions to detect error types

func isDatabaseError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	indicators := []string{
		"column",
		"table",
		"database",
		"connection",
		"sqlstate",
		"constraint",
		"foreign key",
	}

	for _, indicator := range indicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}

	return false
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "validation") ||
		strings.Contains(errMsg, "required") ||
		strings.Contains(errMsg, "invalid")
}

// Utility functions for handlers

// RespondError sends a standardized error response
func RespondError(c *gin.Context, status int, code ErrorCode, message string) {
	err := NewAppError(status, code, message)
	respondWithError(c, err)
}

// RespondErrorWithDetails sends error with field details
func RespondErrorWithDetails(c *gin.Context, status int, code ErrorCode, message string, details map[string]string) {
	err := NewAppError(status, code, message).WithDetails(details)
	respondWithError(c, err)
}

// RespondErrorWithValidation sends validation error
func RespondErrorWithValidation(c *gin.Context, errors map[string]string) {
	err := NewAppError(http.StatusBadRequest, CodeValidationError, "Validation failed").WithDetails(errors)
	respondWithError(c, err)
}

// RespondSuccess sends success response (helper for consistency)
func RespondSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Request successful",
		"data":    data,
	})
}
