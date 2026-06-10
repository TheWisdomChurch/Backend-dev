package middleware

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/apperror"
)

// ErrorHandlerMiddleware provides enterprise-grade error handling
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Only handle errors that weren't already handled
		if len(c.Errors) == 0 {
			return
		}

		// Get the last error
		ginErr := c.Errors.Last()
		var appErr *apperror.AppError
		var ok bool

		// Check if it's an AppError
		if appErr, ok = apperror.As(ginErr.Err); !ok {
			// Not an AppError - wrap it
			appErr = handleUnknownError(ginErr.Err)
		}

		// Log the error
		logError(c, appErr)

		// Send response
		sendErrorResponse(c, appErr)
	}
}

// handleUnknownError wraps unknown errors appropriately
func handleUnknownError(err error) *apperror.AppError {
	if err == nil {
		return apperror.Internal("unknown error")
	}

	errMsg := strings.ToLower(err.Error())

	// Detect error type and wrap appropriately
	if strings.Contains(errMsg, "column") || strings.Contains(errMsg, "sqlstate") {
		return apperror.Wrap(err, fmt.Sprintf("Database error: %v", err))
	}

	if strings.Contains(errMsg, "validation") || strings.Contains(errMsg, "required") {
		return apperror.BadRequest(err.Error())
	}

	// Generic internal error
	return apperror.Wrap(err, err.Error())
}

// sendErrorResponse sends a standardized error response
func sendErrorResponse(c *gin.Context, appErr *apperror.AppError) {
	response := gin.H{
		"status":  "error",
		"message": appErr.Message,
		"code":    appErr.Code,
	}

	// Add request ID if available
	if reqID := c.GetString("request_id"); reqID != "" {
		response["request_id"] = reqID
	}

	// Add timestamp
	response["timestamp"] = time.Now().Format(time.RFC3339)

	// Add field errors if present
	if len(appErr.Fields) > 0 {
		fieldErrors := make(map[string]string)
		for _, fe := range appErr.Fields {
			fieldErrors[fe.Field] = fe.Message
		}
		response["errors"] = fieldErrors
	}

	// Add helpful metadata for specific errors
	response["meta"] = getErrorMetadata(appErr)

	c.JSON(appErr.HTTPStatus, response)
}

// getErrorMetadata returns helpful context for error types
func getErrorMetadata(appErr *apperror.AppError) map[string]interface{} {
	meta := make(map[string]interface{})

	switch appErr.Code {
	case "too_many_requests":
		meta["retry_after"] = 1
		meta["help"] = "You're making requests too quickly. Please wait a moment."

	case "unauthorized":
		meta["help"] = "Your session has expired. Please log in again."
		meta["action"] = "redirect_to_login"

	case "forbidden":
		meta["help"] = "You don't have permission to access this resource."

	case "validation_error":
		meta["help"] = "Please check your input and try again."

	case "internal_error":
		meta["help"] = "An unexpected error occurred. Please contact support if this persists."
		meta["support_contact"] = "support@wisdomchurchhq.org"

	case "not_found":
		meta["help"] = "The requested resource was not found."

	}

	return meta
}

// logError logs errors with full context
func logError(c *gin.Context, appErr *apperror.AppError) {
	requestID := c.GetString("request_id")
	if requestID == "" {
		requestID = c.GetHeader("X-Request-ID")
	}

	userID := c.GetString("user_id")

	logEntry := fmt.Sprintf(
		"[%s] %s %s - %s (User: %s, Code: %s, Internal: %s)",
		requestID,
		c.Request.Method,
		c.Request.URL.Path,
		appErr.Message,
		userID,
		appErr.Code,
		appErr.Internal,
	)

	// Log based on HTTP status
	switch {
	case appErr.HTTPStatus >= 500:
		log.Printf("ERROR: %s", logEntry)
	case appErr.HTTPStatus >= 400:
		log.Printf("WARNING: %s", logEntry)
	default:
		log.Printf("INFO: %s", logEntry)
	}
}

// Helper functions for handlers to use AppError constructors

// ErrorBadRequest returns a bad request error with custom message
func ErrorBadRequest(detail string) *apperror.AppError {
	return apperror.BadRequest(detail)
}

// ErrorValidation returns a validation error with field errors
func ErrorValidation(fields []apperror.FieldError) *apperror.AppError {
	return apperror.ValidationFailed(fields)
}

// ErrorUnauthorized returns an unauthorized error
func ErrorUnauthorized(detail string) *apperror.AppError {
	return apperror.Unauthorized(detail)
}

// ErrorForbidden returns a forbidden error
func ErrorForbidden(detail string) *apperror.AppError {
	return apperror.Forbidden(detail)
}

// ErrorNotFound returns a not found error
func ErrorNotFound(detail string) *apperror.AppError {
	return apperror.NotFound(detail)
}

// ErrorConflict returns a conflict error
func ErrorConflict(detail string) *apperror.AppError {
	return apperror.Conflict(detail)
}

// ErrorInternal returns an internal server error
func ErrorInternal(detail string) *apperror.AppError {
	return apperror.Internal(detail)
}

// ErrorWrap wraps an error as internal server error
func ErrorWrap(err error, detail string) *apperror.AppError {
	return apperror.Wrap(err, detail)
}
