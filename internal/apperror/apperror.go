package apperror

import "net/http"

// FieldError describes a single field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AppError is a typed application error that carries an HTTP status, a
// client-safe message, an internal detail (never exposed to clients), and
// optional field-level validation errors.
type AppError struct {
	Code       string
	Message    string
	Internal   string
	HTTPStatus int
	Fields     []FieldError
	cause      error
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.cause }

// As performs an errors.As-style check and returns the *AppError if found.
func As(err error) (*AppError, bool) {
	if err == nil {
		return nil, false
	}
	for {
		if ae, ok := err.(*AppError); ok {
			return ae, true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return nil, false
}

// ---- constructors ----

func NotFound(detail string) *AppError {
	return &AppError{Code: "not_found", Message: "Resource not found", Internal: detail, HTTPStatus: http.StatusNotFound}
}

func Unauthorized(detail string) *AppError {
	return &AppError{Code: "unauthorized", Message: "Authentication required", Internal: detail, HTTPStatus: http.StatusUnauthorized}
}

func Forbidden(detail string) *AppError {
	return &AppError{Code: "forbidden", Message: "You do not have permission to perform this action", Internal: detail, HTTPStatus: http.StatusForbidden}
}

func BadRequest(detail string) *AppError {
	return &AppError{Code: "bad_request", Message: detail, Internal: detail, HTTPStatus: http.StatusBadRequest}
}

func Conflict(detail string) *AppError {
	return &AppError{Code: "conflict", Message: detail, Internal: detail, HTTPStatus: http.StatusConflict}
}

func Internal(detail string) *AppError {
	return &AppError{Code: "internal_error", Message: "An unexpected error occurred", Internal: detail, HTTPStatus: http.StatusInternalServerError}
}

func ValidationFailed(fields []FieldError) *AppError {
	return &AppError{Code: "validation_error", Message: "Validation failed", HTTPStatus: http.StatusUnprocessableEntity, Fields: fields}
}

// Wrap wraps an existing error as an internal server error, preserving the
// original error in the cause chain for logging.
func Wrap(err error, detail string) *AppError {
	return &AppError{Code: "internal_error", Message: "An unexpected error occurred", Internal: detail, HTTPStatus: http.StatusInternalServerError, cause: err}
}

// WithMessage returns a copy of the error with a custom client-facing message.
func (e *AppError) WithMessage(msg string) *AppError {
	copy := *e
	copy.Message = msg
	return &copy
}

// WithCode returns a copy of the error with a custom machine-readable code.
func (e *AppError) WithCode(code string) *AppError {
	copy := *e
	copy.Code = code
	return &copy
}
