package sync

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestRecordSeen_UpsertsRow(t *testing.T) {
	database, _ := setupEngineTest(t)
	ctx := context.Background()
	repoID := insertTestRepo(t, database, "r1")

	if err := RecordSeen(ctx, database, repoID, "wf-a"); err != nil {
		t.Fatalf("RecordSeen: %v", err)
	}
	if err := RecordSeen(ctx, database, repoID, "wf-a"); err != nil {
		t.Fatalf("RecordSeen retry: %v", err)
	}
	var count int
	_ = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM git_repo_workflows WHERE repo_id = $1 AND workflow_name = 'wf-a'`,
		repoID,
	).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 row, got %d", count)
	}
}

func TestListStale_ReturnsUnseenNames(t *testing.T) {
	database, _ := setupEngineTest(t)
	ctx := context.Background()
	repoID := insertTestRepo(t, database, "r2")

	_ = RecordSeen(ctx, database, repoID, "wf-a")
	_ = RecordSeen(ctx, database, repoID, "wf-b")
	time.Sleep(50 * time.Millisecond)
	syncStart := time.Now()
	time.Sleep(50 * time.Millisecond)
	_ = RecordSeen(ctx, database, repoID, "wf-a") // touched in this sync window

	stale, err := ListStale(ctx, database, repoID, syncStart)
	if err != nil {
		t.Fatalf("ListStale: %v", err)
	}
	if len(stale) != 1 || stale[0] != "wf-b" {
		t.Errorf("stale: got %+v, want [wf-b]", stale)
	}
}

func TestRemoveSeenRecords_DeletesByName(t *testing.T) {
	database, _ := setupEngineTest(t)
	ctx := context.Background()
	repoID := insertTestRepo(t, database, "r3")
	_ = RecordSeen(ctx, database, repoID, "wf-a")

	if err := RemoveSeenRecords(ctx, database, repoID, []string{"wf-a"}); err != nil {
		t.Fatalf("RemoveSeenRecords: %v", err)
	}
	var count int
	_ = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM git_repo_workflows WHERE repo_id = $1`,
		repoID,
	).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after removal, got %d", count)
	}
}

func insertTestRepo(t *testing.T, database *sql.DB, name string) string {
	t.Helper()
	var id string
	err := database.QueryRow(
		`INSERT INTO git_repos (name, url, credential)
		 VALUES ($1, 'https://example.com/r.git', 'pat')
		 RETURNING id`,
		name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert test repo: %v", err)
	}
	return id
}
