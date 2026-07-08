// Package realtime provides a Server-Sent Events hub for pushing events to
// connected admin clients. Each client is identified by user ID. Multiple
// connections from the same user are all fanned out to.
//
// When a Redis client is provided, the hub subscribes to the "wisdomhouse:sse"
// channel and forwards incoming payloads to all local clients. This enables
// horizontal fan-out across multiple server instances without an external
// message broker.
package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	applog "wisdomHouse-backend/internal/logger"
)

const (
	redisPubSubChannel = "wisdomhouse:sse"
	clientBufSize      = 64
	heartbeatInterval  = 25 * time.Second
)

// Event is the payload written to each SSE client.
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// Subscriber is a channel that receives serialised SSE data lines.
type Subscriber chan []byte

// Publisher is implemented by types that can publish to the hub.
type Publisher interface {
	Publish(ctx context.Context, event Event)
}

// RedisPublisher is the minimal Redis interface required for pub/sub.
type RedisPublisher interface {
	Publish(ctx context.Context, channel string, message interface{}) error
	Subscribe(ctx context.Context, channels ...string) RedisSubscription
}

// RedisSubscription is the minimal interface for a Redis pub/sub subscription.
type RedisSubscription interface {
	Channel() <-chan RedisMessage
	Close() error
}

// RedisMessage carries a received pub/sub message.
type RedisMessage interface {
	Payload() string
}

// Hub manages connected SSE clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[string][]Subscriber // userID → subscribers

	redis RedisPublisher
}

// New creates a new Hub. redis may be nil — in that case pub/sub cross-instance
// fan-out is disabled and events are only broadcast to local clients.
func New(redis RedisPublisher) *Hub {
	h := &Hub{
		clients: make(map[string][]Subscriber),
		redis:   redis,
	}
	return h
}

// Start begins the Redis subscription loop. It blocks until ctx is cancelled.
// Call it in a background goroutine.
func (h *Hub) Start(ctx context.Context) {
	if h.redis == nil {
		return
	}
	sub := h.redis.Subscribe(ctx, redisPubSubChannel)
	defer func() { _ = sub.Close() }()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			h.broadcastRaw([]byte(msg.Payload()))
		}
	}
}

// Subscribe registers a new client for userID and returns a channel that
// receives serialised SSE data frames. The caller must call Unsubscribe when
// done.
func (h *Hub) Subscribe(userID string) Subscriber {
	sub := make(Subscriber, clientBufSize)
	h.mu.Lock()
	h.clients[userID] = append(h.clients[userID], sub)
	h.mu.Unlock()
	return sub
}

// Unsubscribe removes a subscriber and closes its channel.
func (h *Hub) Unsubscribe(userID string, sub Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.clients[userID]
	for i, s := range subs {
		if s == sub {
			h.clients[userID] = append(subs[:i], subs[i+1:]...)
			close(sub)
			break
		}
	}
	if len(h.clients[userID]) == 0 {
		delete(h.clients, userID)
	}
}

// Publish sends an event to all local subscribers and, when Redis is configured,
// to all other instances via pub/sub.
func (h *Hub) Publish(ctx context.Context, event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		applog.L().Warn("realtime: marshal event failed", "error", err)
		return
	}
	frame := sseFrame(data)
	h.broadcastRaw(frame)

	if h.redis != nil {
		if err := h.redis.Publish(ctx, redisPubSubChannel, frame); err != nil {
			applog.L().Warn("realtime: redis publish failed", "error", err)
		}
	}
}

// broadcastRaw sends a raw SSE frame to every local subscriber, non-blocking.
func (h *Hub) broadcastRaw(frame []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, subs := range h.clients {
		for _, sub := range subs {
			select {
			case sub <- frame:
			default:
				// Slow consumer — drop frame rather than block.
			}
		}
	}
}

// sseFrame wraps JSON payload in an SSE data frame.
func sseFrame(data []byte) []byte {
	return append(append([]byte("data: "), data...), '\n', '\n')
}

// HeartbeatFrame is sent periodically to keep the connection alive through
// proxies and load balancers that close idle connections.
var HeartbeatFrame = []byte(": heartbeat\n\n")

// HeartbeatInterval is how often the heartbeat is sent to each client.
var HeartbeatInterval = heartbeatInterval
