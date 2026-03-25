package artifact

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Artifact represents metadata for a persisted artifact.
type Artifact struct {
	ID          string
	ExecutionID string
	StepName    string
	Name        string
	URL         string
	Size        int64
	CreatedAt   time.Time
}

// Store manages artifact metadata in Postgres.
type Store struct {
	DB *sql.DB
}

func (s *Store) Create(ctx context.Context, a *Artifact) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO execution_artifacts (execution_id, step_name, name, url, size)
		VALUES ($1, $2, $3, $4, $5)
	`, a.ExecutionID, a.StepName, a.Name, a.URL, a.Size)
	if err != nil {
		return fmt.Errorf("artifact create: %w", err)
	}
	return nil
}

func (s *Store) GetByName(ctx context.Context, executionID, name string) (*Artifact, error) {
	var a Artifact
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, execution_id, step_name, name, url, size, created_at
		FROM execution_artifacts
		WHERE execution_id = $1 AND name = $2
	`, executionID, name).Scan(&a.ID, &a.ExecutionID, &a.StepName, &a.Name, &a.URL, &a.Size, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("artifact %q not found for execution %s", name, executionID)
	}
	if err != nil {
		return nil, fmt.Errorf("artifact get: %w", err)
	}
	return &a, nil
}

func (s *Store) ListByExecution(ctx context.Context, executionID string) ([]Artifact, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, execution_id, step_name, name, url, size, created_at
		FROM execution_artifacts
		WHERE execution_id = $1
		ORDER BY created_at ASC
	`, executionID)
	if err != nil {
		return nil, fmt.Errorf("artifact list: %w", err)
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.ExecutionID, &a.StepName, &a.Name, &a.URL, &a.Size, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("artifact scan: %w", err)
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

// DeleteByExecution removes all artifact metadata for an execution.
func (s *Store) DeleteByExecution(ctx context.Context, executionID string) error {
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM execution_artifacts WHERE execution_id = $1
	`, executionID)
	if err != nil {
		return fmt.Errorf("artifact delete: %w", err)
	}
	return nil
}

// ListExpired returns artifacts older than the given duration.
func (s *Store) ListExpired(ctx context.Context, olderThan time.Duration) ([]Artifact, error) {
	cutoff := time.Now().Add(-olderThan)
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, execution_id, step_name, name, url, size, created_at
		FROM execution_artifacts
		WHERE created_at < $1
		ORDER BY created_at ASC
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("artifact list expired: %w", err)
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.ExecutionID, &a.StepName, &a.Name, &a.URL, &a.Size, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("artifact scan: %w", err)
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}
