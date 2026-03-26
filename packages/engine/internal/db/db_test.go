package db

import (
	"context"
	"testing"

	"github.com/dvflw/mantle/internal/config"
)

func TestOpen_InvalidURL(t *testing.T) {
	_, err := Open(config.DatabaseConfig{URL: "not-a-valid-url"})
	if err == nil {
		t.Fatal("Open() expected error for invalid URL, got nil")
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	if got := FromContext(ctx); got != nil {
		t.Errorf("FromContext(empty) = %v, want nil", got)
	}

	ctx = WithContext(ctx, nil)
	got := FromContext(ctx)
	if got != nil {
		t.Errorf("FromContext(with nil) = %v, want nil", got)
	}
}
