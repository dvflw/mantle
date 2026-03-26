package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Setup configures the global slog default logger to emit structured JSON
// to stdout at the given level. Call this early in process startup.
func Setup(level string) {
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(handler))
}

// parseLevel converts a level string to slog.Level.
// Unrecognised values default to info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
