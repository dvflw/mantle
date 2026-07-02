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
// one version with triggers pointing at another.
func syncTriggersTx(ctx context.Context, tx *sql.Tx, teamID, name string, version int, triggers []Trigger) error {
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
			 (workflow_name, workflow_version, type, schedule, path, secret, team_id, mailbox, folder, filter, poll_interval)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			name, version, t.Type,
			nullableString(t.Schedule), nullableString(t.Path), nullableString(t.Secret), teamID,
			nullableString(t.Mailbox), nullableString(t.Folder), nullableString(t.Filter), nullableString(t.PollInterval),
		); err != nil {
			return fmt.Errorf("registering %s trigger for %q: %w", t.Type, name, err)
		}
	}

	return nil
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
