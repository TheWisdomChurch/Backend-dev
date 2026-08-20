package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/realtime"
)

type SSEHandler struct {
	hub *realtime.Hub
}

func NewSSEHandler(hub *realtime.Hub) *SSEHandler {
	return &SSEHandler{hub: hub}
}

// Stream opens a persistent SSE connection for the authenticated user.
// The client must handle reconnection on disconnect (standard EventSource behaviour).
//
// Headers set:
//   - Content-Type: text/event-stream
//   - Cache-Control: no-cache
//   - X-Accel-Buffering: no  (disables nginx proxy buffering)
//   - Connection: keep-alive
func (h *SSEHandler) Stream(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "unauthorized"})
		return
	}
	userID, _ := userIDVal.(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "unauthorized"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	sub := h.hub.Subscribe(userID)
	defer h.hub.Unsubscribe(userID, sub)

	heartbeat := time.NewTicker(realtime.HeartbeatInterval)
	defer heartbeat.Stop()

	// Send a connected event immediately.
	_, _ = c.Writer.Write([]byte("data: {\"type\":\"connected\"}\n\n"))
	c.Writer.Flush()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case frame, ok := <-sub:
			if !ok {
				return
			}
			_, err := c.Writer.Write(frame)
			if err != nil {
				return
			}
			c.Writer.Flush()
		case <-heartbeat.C:
			_, err := c.Writer.Write(realtime.HeartbeatFrame)
			if err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}
