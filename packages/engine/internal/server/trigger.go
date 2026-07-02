package server

import (
	"context"
	"database/sql"
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

	// Email trigger fields (populated only when Type == "email").
	Mailbox      string
	Folder       string
	Filter       string
	PollInterval string
}

// Trigger registration lives in the workflow package (workflow.Save), which
// reconciles workflow_triggers whenever a definition is applied. The functions
// below are the read side consumed by the cron scheduler, webhook handler, and
// email poller.

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

// LookupWebhookTrigger finds the enabled webhook trigger for a path. Webhook
// paths are a global routing key — an inbound POST /hooks/<path> carries no team
// identity — so the lookup is not team-scoped and the path index is unique
// across all teams. The owning team is read from the matched row so the caller
// can run the workflow under the correct tenant.
func LookupWebhookTrigger(ctx context.Context, db *sql.DB, path string) (*TriggerRecord, error) {
	var t TriggerRecord
	err := db.QueryRowContext(ctx,
		`SELECT id, workflow_name, workflow_version, type, COALESCE(schedule, ''), COALESCE(path, ''), COALESCE(secret, ''), enabled, team_id
		 FROM workflow_triggers WHERE type = 'webhook' AND path = $1 AND enabled = true`, path,
	).Scan(&t.ID, &t.WorkflowName, &t.WorkflowVersion, &t.Type, &t.Schedule, &t.Path, &t.Secret, &t.Enabled, &t.TeamID)
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

// ListEmailTriggers returns all enabled email triggers, including email-specific fields.
func ListEmailTriggers(ctx context.Context, db *sql.DB) ([]TriggerRecord, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, workflow_name, workflow_version, type,
		        COALESCE(mailbox, ''), COALESCE(folder, 'INBOX'),
		        COALESCE(filter, 'unseen'), COALESCE(poll_interval, '60s'),
		        enabled, team_id
		 FROM workflow_triggers WHERE type = 'email' AND enabled = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmailTriggers(rows)
}

// scanEmailTriggers scans rows from ListEmailTriggers (10 columns:
// id, workflow_name, workflow_version, type, mailbox, folder, filter, poll_interval, enabled, team_id).
func scanEmailTriggers(rows *sql.Rows) ([]TriggerRecord, error) {
	var triggers []TriggerRecord
	for rows.Next() {
		var t TriggerRecord
		if err := rows.Scan(
			&t.ID, &t.WorkflowName, &t.WorkflowVersion, &t.Type,
			&t.Mailbox, &t.Folder, &t.Filter, &t.PollInterval,
			&t.Enabled, &t.TeamID,
		); err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}
