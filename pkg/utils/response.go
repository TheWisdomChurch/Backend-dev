package utils

import (
    "github.com/gin-gonic/gin"
    "math"
)

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
    Page       int `json:"page"`
    Limit      int `json:"limit"`
    TotalItems int `json:"total_items"`
    TotalPages int `json:"total_pages"`
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
            Status:  "success",
            Data:    data,
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
