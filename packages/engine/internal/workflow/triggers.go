package workflow

import (
	"context"
	"database/sql"
	"fmt"
)

// syncTriggersTx reconciles the workflow_triggers rows for a workflow within an
// existing transaction. It removes the workflow's current triggers (scoped to
// the team) and inserts the declared set, all enabled. This is what makes
// cron/webhook/email triggers declared in a workflow's YAML actually fire:
// the cron scheduler, webhook handler, and email poller all read from
// workflow_triggers, and without this reconciliation that table stays empty.
//
// It runs inside Save's transaction so a new definition version and its
// triggers are committed atomically — a workflow is never left registered at
// one version with triggers pointing at another. enabled sets the rows'
// enabled flag: true for an active workflow, false when backfilling a
// currently-disabled one so it does not start firing.
func syncTriggersTx(ctx context.Context, tx *sql.Tx, teamID, name string, version int, triggers []Trigger, enabled bool) error {
	// A new version supersedes the previous one, so replace whatever was
	// registered before rather than trying to diff. Scoped to team so we
	// never touch another tenant's rows.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM workflow_triggers WHERE workflow_name = $1 AND team_id = $2`,
		name, teamID); err != nil {
		return fmt.Errorf("clearing triggers for %q: %w", name, err)
	}

	for _, t := range triggers {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO workflow_triggers
			 (workflow_name, workflow_version, type, schedule, path, secret, team_id, mailbox, folder, filter, poll_interval, enabled)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			name, version, t.Type,
			nullableString(t.Schedule), nullableString(t.Path), nullableString(t.Secret), teamID,
			nullableString(t.Mailbox), nullableString(t.Folder), nullableString(t.Filter), nullableString(t.PollInterval),
			enabled,
		); err != nil {
			return fmt.Errorf("registering %s trigger for %q: %w", t.Type, name, err)
		}
	}

	return nil
}

// reconcileTriggersIfDrifted backfills a workflow's triggers on the
// unchanged-content apply path. Save early-returns when the content hash is
// unchanged, so without this a workflow applied before trigger registration
// existed (definition present, workflow_triggers empty) would never register
// its triggers unless the user made a byte-level YAML change. It reconciles
// only when the stored rows are actually out of sync with the latest version's
// declared triggers, so steady-state GitOps re-applies stay churn-free (which
// matters for the email poller, whose IMAP connections key off trigger IDs).
// The backfilled rows inherit the workflow's current enabled/disabled state.
func reconcileTriggersIfDrifted(ctx context.Context, database *sql.DB, teamID, name string, desired []Trigger) error {
	// Latest version and whether it is currently active (not disabled).
	var version int
	var active bool
	err := database.QueryRowContext(ctx,
		`SELECT version, disabled_at IS NULL FROM workflow_definitions
		 WHERE name = $1 AND team_id = $2 ORDER BY version DESC LIMIT 1`,
		name, teamID).Scan(&version, &active)
	if err == sql.ErrNoRows {
		return nil // no definition to reconcile against
	}
	if err != nil {
		return fmt.Errorf("loading latest version for %q: %w", name, err)
	}

	// Already in sync when the row count matches and every row points at the
	// latest version. Covers both the empty-table (pre-fix) case and rows
	// left at a stale version.
	var total, atVersion int
	if err := database.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE workflow_version = $3)
		 FROM workflow_triggers WHERE workflow_name = $1 AND team_id = $2`,
		name, teamID, version).Scan(&total, &atVersion); err != nil {
		return fmt.Errorf("checking trigger drift for %q: %w", name, err)
	}
	if total == len(desired) && atVersion == len(desired) {
		return nil
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := syncTriggersTx(ctx, tx, teamID, name, version, desired, active); err != nil {
		return err
	}
	return tx.Commit()
}

// nullableString maps an empty string to NULL so that nullable TEXT columns
// receive NULL rather than an empty string. Reads COALESCE these back to
// their defaults.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
