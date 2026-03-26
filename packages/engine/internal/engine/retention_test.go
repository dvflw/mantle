package engine

import (
	"context"
	"testing"
)

func TestCleanupExecutions_ZeroDays(t *testing.T) {
	count, err := CleanupExecutions(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestCleanupExecutions_NegativeDays(t *testing.T) {
	count, err := CleanupExecutions(context.Background(), nil, -5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestCleanupAuditEvents_ZeroDays(t *testing.T) {
	count, err := CleanupAuditEvents(context.Background(), nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestCleanupAuditEvents_NegativeDays(t *testing.T) {
	count, err := CleanupAuditEvents(context.Background(), nil, -10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}
