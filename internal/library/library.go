package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Template represents a shared workflow template in the library.
type Template struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Content     json.RawMessage `json:"content"`
	PublishedBy string          `json:"published_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Publish inserts or updates a shared workflow template.
func Publish(ctx context.Context, database *sql.DB, name, description string, content json.RawMessage, teamID string) error {
	_, err := database.ExecContext(ctx,
		`INSERT INTO shared_workflows (name, description, content, published_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (name) DO UPDATE
		 SET description = EXCLUDED.description,
		     content = EXCLUDED.content,
		     published_by = EXCLUDED.published_by,
		     updated_at = NOW()`,
		name, description, content, teamID,
	)
	if err != nil {
		return fmt.Errorf("publishing template %q: %w", name, err)
	}
	return nil
}

// List returns all shared workflow templates (without full content).
func List(ctx context.Context, database *sql.DB) ([]Template, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT id, name, COALESCE(description, ''), COALESCE(published_by::text, '') , created_at
		 FROM shared_workflows
		 ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.PublishedBy, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning template row: %w", err)
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// Get returns a single shared workflow template by name, including its full content.
func Get(ctx context.Context, database *sql.DB, name string) (*Template, error) {
	var t Template
	err := database.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(description, ''), content, COALESCE(published_by::text, ''), created_at
		 FROM shared_workflows
		 WHERE name = $1`,
		name,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Content, &t.PublishedBy, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("template %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("getting template %q: %w", name, err)
	}
	return &t, nil
}

// Deploy copies a shared template into workflow_definitions as version 1 for the target team.
// If the workflow already exists for that team, it appends a new version instead.
// Returns the version number that was created.
func Deploy(ctx context.Context, database *sql.DB, templateName, targetTeamID string) (int, error) {
	tmpl, err := Get(ctx, database, templateName)
	if err != nil {
		return 0, err
	}

	// Determine the next version for this workflow name in the target team.
	var maxVersion int
	err = database.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM workflow_definitions WHERE name = $1 AND team_id = $2`,
		tmpl.Name, targetTeamID,
	).Scan(&maxVersion)
	if err != nil {
		return 0, fmt.Errorf("querying max version: %w", err)
	}
	newVersion := maxVersion + 1

	// Compute a content hash for the deployed definition.
	contentHash := fmt.Sprintf("library-deploy-%s-v%d", templateName, newVersion)

	_, err = database.ExecContext(ctx,
		`INSERT INTO workflow_definitions (name, version, content, content_hash, team_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		tmpl.Name, newVersion, tmpl.Content, contentHash, targetTeamID,
	)
	if err != nil {
		return 0, fmt.Errorf("deploying template %q: %w", templateName, err)
	}

	return newVersion, nil
}
