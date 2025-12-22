package middleware

import (
    "log"
    "time"

    "github.com/gin-gonic/gin"
)

func Logger() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        raw := c.Request.URL.RawQuery

        c.Next()

        timestamp := time.Now()
        latency := timestamp.Sub(start)

        if raw != "" {
            path = path + "?" + raw
        }

        log.Printf("[%s] %s %s %d %s",
            c.Request.Method,
            path,
            c.ClientIP(),
            c.Writer.Status(),
            latency,
        )
    }
}