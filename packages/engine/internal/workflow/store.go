package workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/auth"
)

// Save validates, hashes, and stores a workflow definition as a new immutable version.
// It returns the new version number, or 0 if the content is unchanged from the latest version.
func Save(ctx context.Context, database *sql.DB, result *ParseResult, rawContent []byte) (int, error) {
	errs := Validate(result)
	if len(errs) > 0 {
		return 0, fmt.Errorf("workflow validation failed: %v", errs[0])
	}

	name := result.Workflow.Name
	teamID := auth.TeamIDFromContext(ctx)

	hash := sha256Hash(rawContent)

	latestHash, err := GetLatestHash(ctx, database, name)
	if err != nil {
		return 0, fmt.Errorf("checking latest hash: %w", err)
	}
	if latestHash == hash {
		return 0, nil
	}

	latestVersion, err := GetLatestVersion(ctx, database, name)
	if err != nil {
		return 0, fmt.Errorf("getting latest version: %w", err)
	}
	newVersion := latestVersion + 1

	content, err := json.Marshal(result.Workflow)
	if err != nil {
		return 0, fmt.Errorf("marshaling workflow: %w", err)
	}

	_, err = database.ExecContext(ctx,
		`INSERT INTO workflow_definitions (name, version, content, content_hash, team_id) VALUES ($1, $2, $3, $4, $5)`,
		name, newVersion, content, hash, teamID,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting workflow definition: %w", err)
	}

	return newVersion, nil
}

// GetLatestVersion returns the latest version number for a workflow, or 0 if none exist.
func GetLatestVersion(ctx context.Context, database *sql.DB, name string) (int, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var version int
	err := database.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM workflow_definitions WHERE name = $1 AND team_id = $2`,
		name, teamID,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("querying latest version: %w", err)
	}
	return version, nil
}

// GetLatestHash returns the content hash of the latest version for a workflow,
// or an empty string if no versions exist.
func GetLatestHash(ctx context.Context, database *sql.DB, name string) (string, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var hash string
	err := database.QueryRowContext(ctx,
		`SELECT content_hash FROM workflow_definitions WHERE name = $1 AND team_id = $2 ORDER BY version DESC LIMIT 1`,
		name, teamID,
	).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying latest hash: %w", err)
	}
	return hash, nil
}

// GetLatestContent returns the JSON content of the latest version for a workflow,
// or nil if no versions exist.
func GetLatestContent(ctx context.Context, database *sql.DB, name string) ([]byte, int, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var content []byte
	var version int
	err := database.QueryRowContext(ctx,
		`SELECT content, version FROM workflow_definitions WHERE name = $1 AND team_id = $2 ORDER BY version DESC LIMIT 1`,
		name, teamID,
	).Scan(&content, &version)
	if err == sql.ErrNoRows {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("querying latest content: %w", err)
	}
	return content, version, nil
}

// WorkflowSummary holds metadata for a workflow definition listing.
type WorkflowSummary struct {
	Name          string    `json:"name"`
	LatestVersion int       `json:"latest_version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ListWorkflows returns all distinct workflow definitions for the team.
func ListWorkflows(ctx context.Context, database *sql.DB) ([]WorkflowSummary, error) {
	teamID := auth.TeamIDFromContext(ctx)
	rows, err := database.QueryContext(ctx,
		`SELECT name, MAX(version), MIN(created_at), MAX(created_at)
		 FROM workflow_definitions WHERE team_id = $1
		 GROUP BY name ORDER BY name`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing workflows: %w", err)
	}
	defer rows.Close()

	var workflows []WorkflowSummary
	for rows.Next() {
		var ws WorkflowSummary
		if err := rows.Scan(&ws.Name, &ws.LatestVersion, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning workflow: %w", err)
		}
		workflows = append(workflows, ws)
	}
	return workflows, rows.Err()
}

// VersionSummary holds metadata for a single workflow version.
type VersionSummary struct {
	Version     int       `json:"version"`
	ContentHash string    `json:"content_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

// GetVersions returns all versions for a workflow.
func GetVersions(ctx context.Context, database *sql.DB, name string) ([]VersionSummary, error) {
	teamID := auth.TeamIDFromContext(ctx)
	rows, err := database.QueryContext(ctx,
		`SELECT version, content_hash, created_at
		 FROM workflow_definitions WHERE name = $1 AND team_id = $2
		 ORDER BY version DESC`,
		name, teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}
	defer rows.Close()

	var versions []VersionSummary
	for rows.Next() {
		var vs VersionSummary
		if err := rows.Scan(&vs.Version, &vs.ContentHash, &vs.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning version: %w", err)
		}
		versions = append(versions, vs)
	}
	return versions, rows.Err()
}

// GetVersion returns the content of a specific workflow version.
func GetVersion(ctx context.Context, database *sql.DB, name string, version int) ([]byte, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var content []byte
	err := database.QueryRowContext(ctx,
		`SELECT content FROM workflow_definitions WHERE name = $1 AND version = $2 AND team_id = $3`,
		name, version, teamID,
	).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow %q version %d not found", name, version)
	}
	if err != nil {
		return nil, fmt.Errorf("querying version: %w", err)
	}
	return content, nil
}

func sha256Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
