package utils

import (
    "github.com/gin-gonic/gin"
)

type Response struct {
    Success bool        `json:"success"`
    Message string      `json:"message,omitempty"`
    Data    interface{} `json:"data,omitempty"`
    Error   string      `json:"error,omitempty"`
}

type PaginatedResponse struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data"`
    Page    int         `json:"page"`
    Limit   int         `json:"limit"`
    Total   int64       `json:"total"`
}

func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
    c.JSON(statusCode, Response{
        Success: true,
        Message: message,
        Data:    data,
    })
}

func ErrorResponse(c *gin.Context, statusCode int, message string) {
    c.JSON(statusCode, Response{
        Success: false,
        Error:   message,
    })
}

func PaginatedSuccessResponse(c *gin.Context, statusCode int, data interface{}, page, limit int, total int64) {
    c.JSON(statusCode, PaginatedResponse{
        Success: true,
        Data:    data,
        Page:    page,
        Limit:   limit,
        Total:   total,
    })
}