package environment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/auth"
)

// Environment represents a named set of input and env overrides.
type Environment struct {
	ID        string
	Name      string
	Inputs    map[string]any
	Env       map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Store handles CRUD operations for named environments in Postgres.
type Store struct {
	DB *sql.DB
}

// Create stores a new named environment.
func (s *Store) Create(ctx context.Context, name string, inputs map[string]any, env map[string]string) (*Environment, error) {
	teamID := auth.TeamIDFromContext(ctx)

	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("marshaling inputs: %w", err)
	}
	envJSON, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshaling env: %w", err)
	}

	var e Environment
	err = s.DB.QueryRowContext(ctx,
		`INSERT INTO environments (name, team_id, inputs, env)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, created_at, updated_at`,
		name, teamID, inputsJSON, envJSON,
	).Scan(&e.ID, &e.Name, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating environment %q: %w", name, err)
	}

	e.Inputs = inputs
	e.Env = env
	return &e, nil
}

// Get retrieves a named environment by name.
func (s *Store) Get(ctx context.Context, name string) (*Environment, error) {
	teamID := auth.TeamIDFromContext(ctx)

	var e Environment
	var inputsJSON, envJSON []byte
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, inputs, env, created_at, updated_at
		 FROM environments WHERE name = $1 AND team_id = $2`,
		name, teamID,
	).Scan(&e.ID, &e.Name, &inputsJSON, &envJSON, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying environment: %w", err)
	}

	if inputsJSON != nil {
		if err := json.Unmarshal(inputsJSON, &e.Inputs); err != nil {
			return nil, fmt.Errorf("unmarshaling inputs: %w", err)
		}
	}
	if envJSON != nil {
		if err := json.Unmarshal(envJSON, &e.Env); err != nil {
			return nil, fmt.Errorf("unmarshaling env: %w", err)
		}
	}

	return &e, nil
}

// List returns all environments for the current team.
func (s *Store) List(ctx context.Context) ([]Environment, error) {
	teamID := auth.TeamIDFromContext(ctx)

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, created_at, updated_at
		 FROM environments WHERE team_id = $1 ORDER BY name`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}
	defer rows.Close()

	var envs []Environment
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.Name, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning environment: %w", err)
		}
		envs = append(envs, e)
	}
	return envs, rows.Err()
}

// Delete removes a named environment.
func (s *Store) Delete(ctx context.Context, name string) error {
	teamID := auth.TeamIDFromContext(ctx)

	result, err := s.DB.ExecContext(ctx,
		`DELETE FROM environments WHERE name = $1 AND team_id = $2`,
		name, teamID,
	)
	if err != nil {
		return fmt.Errorf("deleting environment %q: %w", name, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("environment %q not found", name)
	}

	return nil
}
