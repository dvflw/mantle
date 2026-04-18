package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
)

// Store handles CRUD operations for registered git repos. Every
// state-changing method emits an audit event in the same transaction
// as the write so audit log and state never drift.
type Store struct {
	DB    *sql.DB
	Actor string // defaults to "cli" when empty
}

func (s *Store) actor() string {
	if s.Actor == "" {
		return "cli"
	}
	return s.Actor
}

// ErrNotFound is returned when a lookup by name does not match a row in
// the current team scope.
var ErrNotFound = errors.New("repo not found")

// CreateParams captures the fields required to register a new repo.
// Fields with empty defaults (Branch, Path, PollInterval) are filled
// in by the caller using the same defaults the `git_repos` table uses.
type CreateParams struct {
	Name         string
	URL          string
	Branch       string
	Path         string
	PollInterval string
	Credential   string
	AutoApply    bool
	Prune        bool
}

// Create inserts a new repo row and emits a repo.added audit event.
func (s *Store) Create(ctx context.Context, p CreateParams) (*Repo, error) {
	if err := ValidateName(p.Name); err != nil {
		return nil, err
	}
	if err := ValidatePollInterval(p.PollInterval); err != nil {
		return nil, err
	}
	if p.URL == "" {
		return nil, fmt.Errorf("repo url is required")
	}
	if p.Credential == "" {
		return nil, fmt.Errorf("credential is required")
	}

	teamID := auth.TeamIDFromContext(ctx)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var r Repo
	err = tx.QueryRowContext(ctx,
		`INSERT INTO git_repos
		 (team_id, name, url, branch, path, poll_interval, credential, auto_apply, prune)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, name, url, branch, path, poll_interval, credential,
		           auto_apply, prune, enabled, created_at, updated_at`,
		teamID, p.Name, p.URL, p.Branch, p.Path, p.PollInterval, p.Credential,
		p.AutoApply, p.Prune,
	).Scan(&r.ID, &r.Name, &r.URL, &r.Branch, &r.Path, &r.PollInterval,
		&r.Credential, &r.AutoApply, &r.Prune, &r.Enabled,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating repo %q: %w", p.Name, err)
	}

	if err := audit.EmitTx(ctx, tx, audit.Event{
		Timestamp: time.Now(),
		Actor:     s.actor(),
		Action:    audit.ActionRepoAdded,
		Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
		TeamID:    teamID,
		Metadata:  map[string]string{"name": p.Name, "url": p.URL},
	}); err != nil {
		return nil, fmt.Errorf("emitting audit event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing repo create: %w", err)
	}
	return &r, nil
}
