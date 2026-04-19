package sync

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RecordSeen upserts a git_repo_workflows row marking that repoID has
// a file producing workflowName in the current sync pass. Idempotent —
// safe to call multiple times for the same (repo, workflow) pair.
func RecordSeen(ctx context.Context, database *sql.DB, repoID, workflowName string) error {
	_, err := database.ExecContext(ctx,
		`INSERT INTO git_repo_workflows (repo_id, workflow_name, last_seen_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (repo_id, workflow_name) DO UPDATE SET last_seen_at = NOW()`,
		repoID, workflowName,
	)
	if err != nil {
		return fmt.Errorf("recording seen workflow %q for repo %s: %w", workflowName, repoID, err)
	}
	return nil
}

// ListStale returns the names of workflows owned by repoID whose
// last_seen_at predates syncStart — i.e., files that used to be in
// the repo but aren't anymore. Callers use this to drive prune.
func ListStale(ctx context.Context, database *sql.DB, repoID string, syncStart time.Time) ([]string, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT workflow_name FROM git_repo_workflows
		 WHERE repo_id = $1 AND last_seen_at < $2
		 ORDER BY workflow_name`,
		repoID, syncStart,
	)
	if err != nil {
		return nil, fmt.Errorf("listing stale workflows for repo %s: %w", repoID, err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scanning stale workflow: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// RemoveSeenRecords deletes git_repo_workflows rows for the given names.
// Called after prune so a reappearing file is treated as a fresh
// attribution rather than inheriting the stale last_seen_at.
func RemoveSeenRecords(ctx context.Context, database *sql.DB, repoID string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	_, err := database.ExecContext(ctx,
		`DELETE FROM git_repo_workflows
		 WHERE repo_id = $1 AND workflow_name = ANY(
		     SELECT unnest($2::text[])
		 )`,
		repoID, names,
	)
	if err != nil {
		return fmt.Errorf("removing stale workflow records for repo %s: %w", repoID, err)
	}
	return nil
}
