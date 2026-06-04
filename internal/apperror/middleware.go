package apperror

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler returns a Gin middleware that processes errors attached to the
// context via c.Error(...). It must be registered BEFORE route handlers so
// that it runs after them (Gin calls Use() middleware in order for the
// before-Next phase but in reverse for the after-Next phase via defer).
// Register it right after CustomRecovery in setupRouter.
func Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		// Use the last error — subsequent errors may be richer.
		err := c.Errors.Last().Err

		// Already written (e.g. by a middleware that called c.AbortWithStatusJSON)?
		if c.Writer.Written() {
			return
		}

		respond(c, err)
	}
}

// respond writes the appropriate JSON error response for the given error.
func respond(c *gin.Context, err error) {
	if ae, ok := As(err); ok {
		if len(ae.Fields) > 0 {
			c.JSON(ae.HTTPStatus, gin.H{
				"status":  "error",
				"message": ae.Message,
				"errors":  ae.Fields,
			})
			return
		}
		c.JSON(ae.HTTPStatus, gin.H{
			"status":     "error",
			"message":    ae.Message,
			"statusCode": ae.HTTPStatus,
		})
		return
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"status":     "error",
			"message":    "Resource not found",
			"statusCode": http.StatusNotFound,
		})
		return
	}

	// Unknown error — log internally, never expose detail.
	slog.Error("unhandled error", "error", err, "path", c.FullPath(), "method", c.Request.Method)
	c.JSON(http.StatusInternalServerError, gin.H{
		"status":     "error",
		"message":    "An unexpected error occurred",
		"statusCode": http.StatusInternalServerError,
	})
}

// PanicRecoveryHandler is a gin.CustomRecovery func that writes a structured
// JSON 500 instead of the default plain-text response.
func PanicRecoveryHandler(c *gin.Context, recovered any) {
	slog.Error("panic recovered", "panic", recovered, "path", c.FullPath(), "method", c.Request.Method)
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"status":     "error",
		"message":    "An unexpected error occurred",
		"statusCode": http.StatusInternalServerError,
	})
}
