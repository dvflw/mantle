package environment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
)

// maxEnvNameLength caps names at the DNS label limit (RFC 1035). Keeping
// them short enough to use as metric labels and URL path segments without
// truncation.
const maxEnvNameLength = 63

// validEnvNamePattern enforces DNS-label-like names (Kubernetes namespace
// convention): lowercase alphanumerics, underscores, and hyphens, starting
// with an alphanumeric. Chosen so names are safe to embed in URLs, log lines,
// metric labels, and filesystem paths without escaping. Length is enforced
// separately in validateName so the error message can cite the cap.
var validEnvNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

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
// Every state-changing method emits an audit event atomically with the write.
type Store struct {
	DB *sql.DB
	// Actor labels audit events emitted by this store (e.g., "cli", "server").
	// Defaults to "cli" when empty.
	Actor string
}

func (s *Store) actor() string {
	if s.Actor == "" {
		return "cli"
	}
	return s.Actor
}

// ErrNotFound is returned when a lookup for a named environment does not
// match a row in the current team scope.
var ErrNotFound = errors.New("environment not found")

// Create stores a new named environment and emits an audit event in the
// same transaction.
func (s *Store) Create(ctx context.Context, name string, inputs map[string]any, env map[string]string) (*Environment, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	teamID := auth.TeamIDFromContext(ctx)

	inputsJSON, envJSON, err := marshalPayload(inputs, env)
	if err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var e Environment
	err = tx.QueryRowContext(ctx,
		`INSERT INTO environments (name, team_id, inputs, env)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, created_at, updated_at`,
		name, teamID, inputsJSON, envJSON,
	).Scan(&e.ID, &e.Name, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating environment %q: %w", name, err)
	}

	if err := audit.EmitTx(ctx, tx, audit.Event{
		Timestamp: time.Now(),
		Actor:     s.actor(),
		Action:    audit.ActionEnvironmentCreated,
		Resource:  audit.Resource{Type: "environment", ID: e.ID},
		TeamID:    teamID,
		Metadata:  map[string]string{"name": name},
	}); err != nil {
		return nil, fmt.Errorf("emitting audit event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing environment create: %w", err)
	}

	e.Inputs = inputs
	e.Env = env
	return &e, nil
}

// Update replaces the inputs and env for an existing named environment,
// preserving id and created_at. Emits an audit event in the same transaction.
func (s *Store) Update(ctx context.Context, name string, inputs map[string]any, env map[string]string) (*Environment, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	teamID := auth.TeamIDFromContext(ctx)

	inputsJSON, envJSON, err := marshalPayload(inputs, env)
	if err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var e Environment
	err = tx.QueryRowContext(ctx,
		`UPDATE environments
		 SET inputs = $3, env = $4, updated_at = NOW()
		 WHERE name = $1 AND team_id = $2
		 RETURNING id, name, created_at, updated_at`,
		name, teamID, inputsJSON, envJSON,
	).Scan(&e.ID, &e.Name, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("updating environment %q: %w", name, err)
	}

	if err := audit.EmitTx(ctx, tx, audit.Event{
		Timestamp: time.Now(),
		Actor:     s.actor(),
		Action:    audit.ActionEnvironmentUpdated,
		Resource:  audit.Resource{Type: "environment", ID: e.ID},
		TeamID:    teamID,
		Metadata:  map[string]string{"name": name},
	}); err != nil {
		return nil, fmt.Errorf("emitting audit event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing environment update: %w", err)
	}

	e.Inputs = inputs
	e.Env = env
	return &e, nil
}

// Get retrieves a named environment by name. Returns ErrNotFound when no
// matching row exists in the current team scope.
func (s *Store) Get(ctx context.Context, name string) (*Environment, error) {
	teamID := auth.TeamIDFromContext(ctx)

	var e Environment
	var inputsJSON, envJSON []byte
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, inputs, env, created_at, updated_at
		 FROM environments WHERE name = $1 AND team_id = $2`,
		name, teamID,
	).Scan(&e.ID, &e.Name, &inputsJSON, &envJSON, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
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

// Delete removes a named environment and emits an audit event in the same
// transaction. Returns ErrNotFound when no matching row exists.
func (s *Store) Delete(ctx context.Context, name string) error {
	teamID := auth.TeamIDFromContext(ctx)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var deletedID string
	err = tx.QueryRowContext(ctx,
		`DELETE FROM environments WHERE name = $1 AND team_id = $2 RETURNING id`,
		name, teamID,
	).Scan(&deletedID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return fmt.Errorf("deleting environment %q: %w", name, err)
	}

	if err := audit.EmitTx(ctx, tx, audit.Event{
		Timestamp: time.Now(),
		Actor:     s.actor(),
		Action:    audit.ActionEnvironmentDeleted,
		Resource:  audit.Resource{Type: "environment", ID: deletedID},
		TeamID:    teamID,
		Metadata:  map[string]string{"name": name},
	}); err != nil {
		return fmt.Errorf("emitting audit event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing environment delete: %w", err)
	}

	return nil
}

// EmitReveal records that an operator viewed raw values of an environment.
// Callers MUST invoke this before printing sensitive values so that reveals
// are captured in the audit log. Opens its own short-lived transaction.
func (s *Store) EmitReveal(ctx context.Context, e *Environment) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting audit transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := audit.EmitTx(ctx, tx, audit.Event{
		Timestamp: time.Now(),
		Actor:     s.actor(),
		Action:    audit.ActionEnvironmentRevealed,
		Resource:  audit.Resource{Type: "environment", ID: e.ID},
		TeamID:    auth.TeamIDFromContext(ctx),
		Metadata:  map[string]string{"name": e.Name},
	}); err != nil {
		return fmt.Errorf("emitting reveal audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing reveal audit event: %w", err)
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("environment name is required")
	}
	if len(name) > maxEnvNameLength {
		return fmt.Errorf("invalid environment name %q: length %d exceeds %d-char cap", name, len(name), maxEnvNameLength)
	}
	if !validEnvNamePattern.MatchString(name) {
		return fmt.Errorf("invalid environment name %q: must match %s", name, validEnvNamePattern.String())
	}
	return nil
}

func marshalPayload(inputs map[string]any, env map[string]string) ([]byte, []byte, error) {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling inputs: %w", err)
	}
	envJSON, err := json.Marshal(env)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling env: %w", err)
	}
	return inputsJSON, envJSON, nil
}
