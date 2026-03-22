package server

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dvflw/mantle/internal/auth"
)

// TriggerRecord represents a registered trigger in the database.
type TriggerRecord struct {
	ID              string
	WorkflowName    string
	WorkflowVersion int
	Type            string
	Schedule        string
	Path            string
	Secret          string
	Enabled         bool
	TeamID          string
}

// SyncTriggers replaces all triggers for a workflow with the given set.
// Called by `mantle apply` when a workflow definition changes.
func SyncTriggers(ctx context.Context, db *sql.DB, workflowName string, version int, triggers []TriggerInput) error {
	teamID := auth.TeamIDFromContext(ctx)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Remove existing triggers for this workflow, scoped to team.
	_, err = tx.ExecContext(ctx,
		`DELETE FROM workflow_triggers WHERE workflow_name = $1 AND team_id = $2`, workflowName, teamID)
	if err != nil {
		return fmt.Errorf("deleting old triggers: %w", err)
	}

	// Insert new triggers.
	for _, t := range triggers {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO workflow_triggers (workflow_name, workflow_version, type, schedule, path, secret, team_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			workflowName, version, t.Type, t.Schedule, t.Path, t.Secret, teamID)
		if err != nil {
			return fmt.Errorf("inserting trigger: %w", err)
		}
	}

	return tx.Commit()
}

// TriggerInput is the data needed to register a trigger.
type TriggerInput struct {
	Type     string
	Schedule string
	Path     string
	Secret   string
}

// ListCronTriggers returns all enabled cron triggers, including team_id for proper scoping.
func ListCronTriggers(ctx context.Context, db *sql.DB) ([]TriggerRecord, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, workflow_name, workflow_version, type, COALESCE(schedule, ''), COALESCE(path, ''), enabled, team_id
		 FROM workflow_triggers WHERE type = 'cron' AND enabled = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTriggers(rows)
}

// LookupWebhookTrigger finds the trigger matching a webhook path, scoped to the team.
func LookupWebhookTrigger(ctx context.Context, db *sql.DB, path string) (*TriggerRecord, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var t TriggerRecord
	err := db.QueryRowContext(ctx,
		`SELECT id, workflow_name, workflow_version, type, COALESCE(schedule, ''), COALESCE(path, ''), COALESCE(secret, ''), enabled
		 FROM workflow_triggers WHERE type = 'webhook' AND path = $1 AND enabled = true AND team_id = $2`, path, teamID,
	).Scan(&t.ID, &t.WorkflowName, &t.WorkflowVersion, &t.Type, &t.Schedule, &t.Path, &t.Secret, &t.Enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTriggers(rows *sql.Rows) ([]TriggerRecord, error) {
	var triggers []TriggerRecord
	for rows.Next() {
		var t TriggerRecord
		if err := rows.Scan(&t.ID, &t.WorkflowName, &t.WorkflowVersion, &t.Type, &t.Schedule, &t.Path, &t.Enabled, &t.TeamID); err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}
