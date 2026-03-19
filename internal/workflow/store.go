package workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Save validates, hashes, and stores a workflow definition as a new immutable version.
// It returns the new version number, or 0 if the content is unchanged from the latest version.
func Save(ctx context.Context, database *sql.DB, result *ParseResult, rawContent []byte) (int, error) {
	errs := Validate(result)
	if len(errs) > 0 {
		return 0, fmt.Errorf("workflow validation failed: %v", errs[0])
	}

	name := result.Workflow.Name

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
		`INSERT INTO workflow_definitions (name, version, content, content_hash) VALUES ($1, $2, $3, $4)`,
		name, newVersion, content, hash,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting workflow definition: %w", err)
	}

	return newVersion, nil
}

// GetLatestVersion returns the latest version number for a workflow, or 0 if none exist.
func GetLatestVersion(ctx context.Context, database *sql.DB, name string) (int, error) {
	var version int
	err := database.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM workflow_definitions WHERE name = $1`,
		name,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("querying latest version: %w", err)
	}
	return version, nil
}

// GetLatestHash returns the content hash of the latest version for a workflow,
// or an empty string if no versions exist.
func GetLatestHash(ctx context.Context, database *sql.DB, name string) (string, error) {
	var hash string
	err := database.QueryRowContext(ctx,
		`SELECT content_hash FROM workflow_definitions WHERE name = $1 ORDER BY version DESC LIMIT 1`,
		name,
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
	var content []byte
	var version int
	err := database.QueryRowContext(ctx,
		`SELECT content, version FROM workflow_definitions WHERE name = $1 ORDER BY version DESC LIMIT 1`,
		name,
	).Scan(&content, &version)
	if err == sql.ErrNoRows {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("querying latest content: %w", err)
	}
	return content, version, nil
}

func sha256Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
