package engine

import (
	"context"
	"testing"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
)

func TestCheckConcurrencyLimits_AllowedWhenUnderLimit(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// Insert 2 running executions for workflow "wf-a".
	for i := 0; i < 2; i++ {
		_, err := database.ExecContext(ctx,
			`INSERT INTO workflow_executions (workflow_name, workflow_version, status)
			 VALUES ('wf-a', 1, 'running')`)
		if err != nil {
			t.Fatalf("inserting running execution: %v", err)
		}
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	result := CheckConcurrencyLimits(ctx, tx, "wf-a", 5, "queue", 0)
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if !result.Allowed {
		t.Errorf("expected Allowed=true, got false (Queued=%v, Err=%v)", result.Queued, result.Err)
	}
	if result.Queued {
		t.Error("expected Queued=false, got true")
	}
	if result.Err != nil {
		t.Errorf("expected nil error, got: %v", result.Err)
	}
}

func TestCheckConcurrencyLimits_QueuedWhenOverLimit(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// Insert 2 running executions for workflow "wf-b".
	for i := 0; i < 2; i++ {
		_, err := database.ExecContext(ctx,
			`INSERT INTO workflow_executions (workflow_name, workflow_version, status)
			 VALUES ('wf-b', 1, 'running')`)
		if err != nil {
			t.Fatalf("inserting running execution: %v", err)
		}
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	result := CheckConcurrencyLimits(ctx, tx, "wf-b", 2, "queue", 0)
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if result.Allowed {
		t.Error("expected Allowed=false, got true")
	}
	if !result.Queued {
		t.Errorf("expected Queued=true, got false (Err=%v)", result.Err)
	}
	if result.Err != nil {
		t.Errorf("expected nil error, got: %v", result.Err)
	}
}

func TestCheckConcurrencyLimits_RejectedWhenOverLimitReject(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// Insert 2 running executions for workflow "wf-c".
	for i := 0; i < 2; i++ {
		_, err := database.ExecContext(ctx,
			`INSERT INTO workflow_executions (workflow_name, workflow_version, status)
			 VALUES ('wf-c', 1, 'running')`)
		if err != nil {
			t.Fatalf("inserting running execution: %v", err)
		}
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	result := CheckConcurrencyLimits(ctx, tx, "wf-c", 2, "reject", 0)
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if result.Err == nil {
		t.Fatal("expected non-nil error for reject policy, got nil")
	}
	if result.Allowed {
		t.Error("expected Allowed=false when rejected")
	}
}

func TestCheckConcurrencyLimits_UnlimitedWhenZero(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// Insert many running executions.
	for i := 0; i < 10; i++ {
		_, err := database.ExecContext(ctx,
			`INSERT INTO workflow_executions (workflow_name, workflow_version, status)
			 VALUES ('wf-d', 1, 'running')`)
		if err != nil {
			t.Fatalf("inserting running execution: %v", err)
		}
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	// maxParallelExecutions=0 means unlimited.
	result := CheckConcurrencyLimits(ctx, tx, "wf-d", 0, "queue", 0)
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if !result.Allowed {
		t.Errorf("expected Allowed=true when max=0 (unlimited), got false (Queued=%v, Err=%v)", result.Queued, result.Err)
	}
}

func TestPromoteQueued_PromotesOldest(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	// Insert 2 queued executions with different started_at times.
	var oldestID, newestID string
	err := database.QueryRowContext(ctx,
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status, started_at)
		 VALUES ('wf-promote', 1, 'queued', NOW() - INTERVAL '10 minutes')
		 RETURNING id`).Scan(&oldestID)
	if err != nil {
		t.Fatalf("inserting oldest queued: %v", err)
	}

	err = database.QueryRowContext(ctx,
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status, started_at)
		 VALUES ('wf-promote', 1, 'queued', NOW() - INTERVAL '1 minute')
		 RETURNING id`).Scan(&newestID)
	if err != nil {
		t.Fatalf("inserting newest queued: %v", err)
	}

	// Promote oldest.
	if err := PromoteQueued(ctx, database, "wf-promote", &audit.NoopEmitter{}); err != nil {
		t.Fatalf("PromoteQueued() error: %v", err)
	}

	// Verify oldest was promoted to pending.
	var oldestStatus, newestStatus string
	if err := database.QueryRowContext(ctx,
		`SELECT status FROM workflow_executions WHERE id = $1`, oldestID).Scan(&oldestStatus); err != nil {
		t.Fatalf("querying oldest status: %v", err)
	}
	if err := database.QueryRowContext(ctx,
		`SELECT status FROM workflow_executions WHERE id = $1`, newestID).Scan(&newestStatus); err != nil {
		t.Fatalf("querying newest status: %v", err)
	}

	if oldestStatus != "pending" {
		t.Errorf("oldest execution status = %q, want %q", oldestStatus, "pending")
	}
	if newestStatus != "queued" {
		t.Errorf("newest execution status = %q, want %q (should remain queued)", newestStatus, "queued")
	}
}

func TestPromoteQueuedByTeam_PromotesOldest(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	teamID := auth.DefaultTeamID

	// Insert 2 queued executions for the default team.
	var oldestID, newestID string
	err := database.QueryRowContext(ctx,
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status, started_at, team_id)
		 VALUES ('wf-team-a', 1, 'queued', NOW() - INTERVAL '10 minutes', $1)
		 RETURNING id`, teamID).Scan(&oldestID)
	if err != nil {
		t.Fatalf("inserting oldest queued: %v", err)
	}
	err = database.QueryRowContext(ctx,
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status, started_at, team_id)
		 VALUES ('wf-team-b', 1, 'queued', NOW() - INTERVAL '1 minute', $1)
		 RETURNING id`, teamID).Scan(&newestID)
	if err != nil {
		t.Fatalf("inserting newest queued: %v", err)
	}

	if err := PromoteQueuedByTeam(ctx, database, teamID, &audit.NoopEmitter{}); err != nil {
		t.Fatalf("PromoteQueuedByTeam() error: %v", err)
	}

	var oldestStatus string
	if err := database.QueryRowContext(ctx,
		`SELECT status FROM workflow_executions WHERE id = $1`, oldestID).Scan(&oldestStatus); err != nil {
		t.Fatalf("querying oldest status: %v", err)
	}
	if oldestStatus != "pending" {
		t.Errorf("oldest execution status = %q, want %q", oldestStatus, "pending")
	}
}

func TestHashString_Returns64Bit(t *testing.T) {
	// Verify deterministic output.
	a := hashString("team:abc")
	b := hashString("team:abc")
	if a != b {
		t.Errorf("hashString not deterministic: %d != %d", a, b)
	}

	// Verify different inputs produce different hashes.
	c := hashString("workflow:xyz")
	if a == c {
		t.Error("hashString returned same value for different inputs")
	}

	// Verify non-zero (extremely unlikely for FNV but good sanity check).
	if a == 0 {
		t.Error("hashString returned 0")
	}
}
