package sync

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/repo"
	"github.com/dvflw/mantle/internal/workflow"
)

// Report is the outcome of a single sync pass over one repo.
type Report struct {
	SHA       string
	Applied   int
	Unchanged int
	Failures  []FileResult
}

// FileResult captures a per-file failure so operators can trace which
// YAML file caused trouble without having to re-run the sync.
type FileResult struct {
	RelPath string
	Err     string
}

// SyncRepo runs one sync cycle for r: pulls via driver, walks the
// configured path, and applies each workflow YAML. Per-file failures
// accumulate in the report but do not abort the sync — the engine's
// contract is "apply everything you can, then tell the operator what
// broke." Emits audit events for start, success, validation errors,
// and final completion. Always updates LastSyncAt on the repo, even
// on partial failure.
func SyncRepo(ctx context.Context, database *sql.DB, store *repo.Store, r *repo.Repo, driver Driver) (*Report, error) {
	teamID := auth.TeamIDFromContext(ctx)

	emit(ctx, database, audit.Event{
		Timestamp: time.Now(),
		Actor:     "sync",
		Action:    audit.ActionGitSyncStarted,
		Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
		TeamID:    teamID,
		Metadata:  map[string]string{"name": r.Name},
	})

	pull, err := driver.Pull(ctx, r)
	if err != nil {
		_ = store.UpdateSyncState(ctx, r.ID, "", fmt.Sprintf("pull failed: %v", err))
		emit(ctx, database, audit.Event{
			Timestamp: time.Now(),
			Actor:     "sync",
			Action:    audit.ActionGitSyncFailed,
			Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
			TeamID:    teamID,
			Metadata:  map[string]string{"name": r.Name, "error": err.Error()},
		})
		return nil, fmt.Errorf("driver pull: %w", err)
	}

	files, err := Discover(pull.LocalPath, r.Path)
	if err != nil {
		_ = store.UpdateSyncState(ctx, r.ID, pull.SHA, fmt.Sprintf("discover failed: %v", err))
		emit(ctx, database, audit.Event{
			Timestamp: time.Now(),
			Actor:     "sync",
			Action:    audit.ActionGitSyncFailed,
			Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
			TeamID:    teamID,
			Metadata:  map[string]string{"name": r.Name, "error": err.Error()},
		})
		return nil, fmt.Errorf("discover: %w", err)
	}

	report := &Report{SHA: pull.SHA}
	for _, f := range files {
		parseResult, parseErr := workflow.ParseBytes(f.Bytes)
		if parseErr != nil {
			report.Failures = append(report.Failures, FileResult{RelPath: f.RelPath, Err: parseErr.Error()})
			emit(ctx, database, audit.Event{
				Timestamp: time.Now(),
				Actor:     "sync",
				Action:    audit.ActionGitSyncValidationError,
				Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
				TeamID:    teamID,
				Metadata:  map[string]string{"name": r.Name, "file": f.RelPath, "error": parseErr.Error()},
			})
			continue
		}
		version, saveErr := workflow.Save(ctx, database, parseResult, f.Bytes)
		if saveErr != nil {
			report.Failures = append(report.Failures, FileResult{RelPath: f.RelPath, Err: saveErr.Error()})
			emit(ctx, database, audit.Event{
				Timestamp: time.Now(),
				Actor:     "sync",
				Action:    audit.ActionGitSyncValidationError,
				Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
				TeamID:    teamID,
				Metadata:  map[string]string{"name": r.Name, "file": f.RelPath, "error": saveErr.Error()},
			})
			continue
		}
		if version == 0 {
			report.Unchanged++
		} else {
			report.Applied++
		}
	}

	errSummary := ""
	if len(report.Failures) > 0 {
		errSummary = summarizeFailures(report.Failures)
	}
	_ = store.UpdateSyncState(ctx, r.ID, pull.SHA, errSummary)

	emit(ctx, database, audit.Event{
		Timestamp: time.Now(),
		Actor:     "sync",
		Action:    audit.ActionGitSyncCompleted,
		Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
		TeamID:    teamID,
		Metadata: map[string]string{
			"name":      r.Name,
			"sha":       pull.SHA,
			"applied":   fmt.Sprintf("%d", report.Applied),
			"unchanged": fmt.Sprintf("%d", report.Unchanged),
			"failures":  fmt.Sprintf("%d", len(report.Failures)),
		},
	})
	return report, nil
}

// emit opens a short-lived transaction for the audit write. Errors are
// swallowed — an audit emission failure must not fail the sync.
func emit(ctx context.Context, database *sql.DB, e audit.Event) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback() //nolint:errcheck
	if err := audit.EmitTx(ctx, tx, e); err != nil {
		return
	}
	_ = tx.Commit()
}

// summarizeFailures joins up to three failure messages into a single
// string suitable for last_sync_error. Audit events carry the full
// detail; this column is for at-a-glance CLI output.
func summarizeFailures(fs []FileResult) string {
	if len(fs) == 0 {
		return ""
	}
	var parts []string
	for i, f := range fs {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("... and %d more", len(fs)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%s: %s", f.RelPath, f.Err))
	}
	return strings.Join(parts, "; ")
}
