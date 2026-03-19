package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{"  info  ", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetup_JSONOutput(t *testing.T) {
	// Capture output by creating a handler with a buffer, similar to what
	// Setup does but writing to a buffer so we can inspect.
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: parseLevel("info"),
	})
	logger := slog.New(handler)

	logger.Info("test message", "key", "value")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}

	if msg, ok := entry["msg"].(string); !ok || msg != "test message" {
		t.Errorf("expected msg=%q, got %v", "test message", entry["msg"])
	}
	if val, ok := entry["key"].(string); !ok || val != "value" {
		t.Errorf("expected key=%q, got %v", "value", entry["key"])
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected time field in JSON output")
	}
	if _, ok := entry["level"]; !ok {
		t.Error("expected level field in JSON output")
	}
}

func TestSetup_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: parseLevel("warn"),
	})
	logger := slog.New(handler)

	logger.Info("should be filtered")
	if buf.Len() != 0 {
		t.Errorf("info message should be filtered at warn level, got: %s", buf.String())
	}

	logger.Warn("should appear")
	if buf.Len() == 0 {
		t.Error("warn message should appear at warn level")
	}
}

func TestSetup_SetsDefault(t *testing.T) {
	Setup("debug")

	// After Setup, the default logger should be configured.
	// We can verify it's a JSON handler by checking that it's enabled for debug.
	if !slog.Default().Enabled(nil, slog.LevelDebug) {
		t.Error("expected debug level to be enabled after Setup(\"debug\")")
	}
}
