package apperror

import "net/http"

// Machine-readable error codes sent in the "code" field of every error response.
// Clients should branch on these rather than HTTP status codes or message strings.
const (
	// Auth / session
	CodeInvalidCredentials = "ERR_INVALID_CREDENTIALS"
	CodeAccountLocked      = "ERR_ACCOUNT_LOCKED"
	CodeAccountInactive    = "ERR_ACCOUNT_INACTIVE"
	CodeAdminPending       = "ERR_ADMIN_PENDING"
	CodeMFARequired        = "ERR_MFA_REQUIRED"
	CodeTokenExpired       = "ERR_TOKEN_EXPIRED"
	CodeTokenInvalid       = "ERR_TOKEN_INVALID"
	CodeSessionExpired     = "ERR_SESSION_EXPIRED"

	// OTP
	CodeOTPExpired = "ERR_OTP_EXPIRED"
	CodeOTPInvalid = "ERR_OTP_INVALID"

	// Password
	CodePasswordPolicy  = "ERR_PASSWORD_POLICY"
	CodeBreachDetected  = "ERR_BREACH_DETECTED"
	CodePasswordHistory = "ERR_PASSWORD_HISTORY"

	// Payments
	CodePaymentFailed  = "ERR_PAYMENT_FAILED"
	CodePaymentPending = "ERR_PAYMENT_PENDING"

	// Pagination
	CodeInvalidCursor = "ERR_INVALID_CURSOR"

	// Encryption / data integrity
	CodeEncryptionFailed = "ERR_ENCRYPTION_FAILED"

	// Rate limiting
	CodeRateLimited = "ERR_RATE_LIMITED"

	// Device trust
	CodeDeviceUntrusted = "ERR_DEVICE_UNTRUSTED"
)

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

func TooManyRequests(detail string) *AppError {
	return &AppError{Code: CodeRateLimited, Message: "Too many requests — please slow down", Internal: detail, HTTPStatus: http.StatusTooManyRequests}
}

func PaymentFailed(detail string) *AppError {
	return &AppError{Code: CodePaymentFailed, Message: "Payment processing failed", Internal: detail, HTTPStatus: http.StatusPaymentRequired}
}

func ServiceUnavailable(detail string) *AppError {
	return &AppError{Code: "service_unavailable", Message: "Service temporarily unavailable", Internal: detail, HTTPStatus: http.StatusServiceUnavailable}
}

func InvalidCursor(detail string) *AppError {
	return &AppError{Code: CodeInvalidCursor, Message: "Invalid pagination cursor", Internal: detail, HTTPStatus: http.StatusBadRequest}
}

// AccountLocked returns a 423 Locked error for locked user accounts.
func AccountLocked(detail string) *AppError {
	return &AppError{Code: CodeAccountLocked, Message: "Account is temporarily locked due to too many failed attempts", Internal: detail, HTTPStatus: http.StatusLocked}
}

// OTPExpired signals that the one-time code has passed its validity window.
func OTPExpired(detail string) *AppError {
	return &AppError{Code: CodeOTPExpired, Message: "Verification code has expired — please request a new one", Internal: detail, HTTPStatus: http.StatusUnauthorized}
}

// OTPInvalid signals that the one-time code is wrong but not expired.
func OTPInvalid(detail string) *AppError {
	return &AppError{Code: CodeOTPInvalid, Message: "Invalid verification code", Internal: detail, HTTPStatus: http.StatusUnauthorized}
}

// PasswordPolicy signals a password that fails strength requirements.
func PasswordPolicy(detail string) *AppError {
	return &AppError{Code: CodePasswordPolicy, Message: detail, Internal: detail, HTTPStatus: http.StatusUnprocessableEntity}
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
