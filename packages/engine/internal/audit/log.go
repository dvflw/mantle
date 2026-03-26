package audit

import (
	"context"
	"log/slog"
)

// LogEmitter emits audit events as structured log messages via slog.
type LogEmitter struct {
	Logger *slog.Logger
}

func (l *LogEmitter) Emit(_ context.Context, event Event) error {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}

	attrs := []any{
		slog.String("action", string(event.Action)),
		slog.String("resource_type", event.Resource.Type),
		slog.String("resource_id", event.Resource.ID),
		slog.String("actor", event.Actor),
		slog.Time("timestamp", event.Timestamp),
	}

	if event.ID != "" {
		attrs = append(attrs, slog.String("event_id", event.ID))
	}
	for k, v := range event.Metadata {
		attrs = append(attrs, slog.String("meta."+k, v))
	}

	logger.Info("[AUDIT]", attrs...)
	return nil
}
