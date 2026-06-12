package utils

import (
	"errors"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/apperror"
)

// ---------------------------------------------------------------------------
// Canonical response envelope (all new code should use these types)
// ---------------------------------------------------------------------------

// APIResponse is the canonical envelope for every API response.
type APIResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError is the machine-readable error payload inside APIResponse.
type APIError struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Fields  []apperror.FieldError `json:"fields,omitempty"`
}

// PageMeta carries offset-based pagination metadata.
type PageMeta struct {
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	Total      int64  `json:"total"`
	TotalPages int    `json:"total_pages"`
	HasNext    bool   `json:"has_next"`
	HasPrev    bool   `json:"has_prev"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// CursorMeta carries cursor-based pagination metadata.
type CursorMeta struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// ---------------------------------------------------------------------------
// Shorthand response helpers
// ---------------------------------------------------------------------------

// OK sends 200 with data.
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Status: "success", Data: data})
}

// OKMsg sends 200 with data and a message.
func OKMsg(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Status: "success", Message: message, Data: data})
}

// Created sends 201 with data.
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{Status: "success", Data: data})
}

// NoContent sends 204.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// OKPage sends 200 with data and offset-based page metadata.
func OKPage(c *gin.Context, data interface{}, meta PageMeta) {
	c.JSON(http.StatusOK, APIResponse{Status: "success", Data: data, Meta: meta})
}

// OKCursor sends 200 with data and cursor-based pagination metadata.
func OKCursor(c *gin.Context, data interface{}, meta CursorMeta) {
	c.JSON(http.StatusOK, APIResponse{Status: "success", Data: data, Meta: meta})
}

// Err maps an error to the appropriate HTTP status and response body.
// If err is an *apperror.AppError the code and status are taken from it;
// otherwise a generic 500 is returned.
func Err(c *gin.Context, err error) {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		c.JSON(ae.HTTPStatus, APIResponse{
			Status: "error",
			Error: &APIError{
				Code:    ae.Code,
				Message: ae.Message,
				Fields:  ae.Fields,
			},
		})
		return
	}
	c.JSON(http.StatusInternalServerError, APIResponse{
		Status: "error",
		Error:  &APIError{Code: "internal_error", Message: "An unexpected error occurred"},
	})
}

// BuildPageMeta constructs a PageMeta from typical pagination parameters.
func BuildPageMeta(page, limit int, total int64) PageMeta {
	totalPages := 0
	if limit > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(limit)))
	}
	return PageMeta{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
}

// ---------------------------------------------------------------------------
// Legacy helpers — kept for backward compatibility; prefer the helpers above
// ---------------------------------------------------------------------------

// Response structure for consistent API responses
type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// PaginatedResponse for paginated data
type PaginatedResponse struct {
	Response
	Meta PaginationMeta `json:"meta"`
}

type PaginationMeta struct {
	Page       int  `json:"page"`
	Limit      int  `json:"limit"`
	TotalItems int  `json:"total_items"`
	TotalPages int  `json:"total_pages"`
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}

// SuccessResponse sends a success response
func SuccessResponse(c *gin.Context, status int, message string, data interface{}) {
	c.JSON(status, Response{
		Status:  "success",
		Message: message,
		Data:    data,
	})
}

// ErrorResponse sends an error response
func ErrorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, Response{
		Status:  "error",
		Message: message,
	})
}

// PaginatedSuccessResponse sends a paginated success response
func PaginatedSuccessResponse(c *gin.Context, status int, data interface{}, page, limit, total int) {
	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	c.JSON(status, PaginatedResponse{
		Response: Response{
			Status: "success",
			Data:   data,
		},
		Meta: PaginationMeta{
			Page:       page,
			Limit:      limit,
			TotalItems: total,
			TotalPages: totalPages,
			HasNext:    page < totalPages,
			HasPrev:    page > 1,
		},
	})
}
