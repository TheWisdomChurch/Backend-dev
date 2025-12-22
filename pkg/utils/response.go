package utils

import (
    

    "github.com/gin-gonic/gin"
)

type Response struct {
    Success bool        `json:"success"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
    Error   string      `json:"error,omitempty"`
}

type PaginatedResponse struct {
    Success  bool        `json:"success"`
    Message  string      `json:"message"`
    Data     interface{} `json:"data,omitempty"`
    Page     int         `json:"page"`
    Limit    int         `json:"limit"`
    Total    int64       `json:"total"`
    LastPage int         `json:"last_page"`
}

func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
    response := Response{
        Success: true,
        Message: message,
        Data:    data,
    }
    c.JSON(statusCode, response)
}

func ErrorResponse(c *gin.Context, statusCode int, message string) {
    response := Response{
        Success: false,
        Message: message,
        Error:   message,
    }
    c.JSON(statusCode, response)
}

func PaginatedSuccessResponse(c *gin.Context, statusCode int, data interface{}, page, limit int, total int64) {
    lastPage := int((total + int64(limit) - 1) / int64(limit))
    
    response := PaginatedResponse{
        Success:  true,
        Message:  "Data fetched successfully",
        Data:     data,
        Page:     page,
        Limit:    limit,
        Total:    total,
        LastPage: lastPage,
    }
    c.JSON(statusCode, response)
}