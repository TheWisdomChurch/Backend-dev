package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type contextKey struct{}

var global *slog.Logger

// Init initialises the global structured logger. Call once at startup before
// any other component uses L() or FromContext().
func Init(level, env string) {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if strings.TrimSpace(env) == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	global = slog.New(handler)
	slog.SetDefault(global)
}

// L returns the global logger, initialising a default one if Init was never
// called (useful in tests).
func L() *slog.Logger {
	if global == nil {
		global = slog.Default()
	}
	return global
}

// WithContext stores a logger in the context so downstream code can retrieve
// it without threading the logger through every function signature.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext retrieves the logger stored by WithContext. Falls back to the
// global logger if none was stored.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return L()
}
