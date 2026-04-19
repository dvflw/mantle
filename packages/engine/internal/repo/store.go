package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
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
	Name          string
	URL           string
	Branch        string
	Path          string
	PollInterval  string
	Credential    string
	AutoApply     bool
	Prune         bool
	WebhookSecret string // empty → NULL (no HMAC verification)
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
	if err := validateURL(p.URL); err != nil {
		return nil, err
	}

	teamID := auth.TeamIDFromContext(ctx)

	var webhookSecret any
	if p.WebhookSecret != "" {
		webhookSecret = p.WebhookSecret
	} else {
		webhookSecret = nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var r Repo
	var returnedWebhookSecret sql.NullString
	err = tx.QueryRowContext(ctx,
		`INSERT INTO git_repos
		 (team_id, name, url, branch, path, poll_interval, credential, auto_apply, prune, webhook_secret)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, name, url, branch, path, poll_interval, credential,
		           auto_apply, prune, enabled, webhook_secret, created_at, updated_at`,
		teamID, p.Name, p.URL, p.Branch, p.Path, p.PollInterval, p.Credential,
		p.AutoApply, p.Prune, webhookSecret,
	).Scan(&r.ID, &r.Name, &r.URL, &r.Branch, &r.Path, &r.PollInterval,
		&r.Credential, &r.AutoApply, &r.Prune, &r.Enabled, &returnedWebhookSecret,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating repo %q: %w", p.Name, err)
	}
	if returnedWebhookSecret.Valid {
		r.WebhookSecret = returnedWebhookSecret.String
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

// List returns all repos in the current team scope, ordered by name.
// Last-sync fields are populated when non-null. The credential name is
// returned but raw secret material is never loaded here.
func (s *Store) List(ctx context.Context) ([]Repo, error) {
	teamID := auth.TeamIDFromContext(ctx)

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, url, branch, path, poll_interval, credential,
		        auto_apply, prune, enabled, last_sync_sha, last_sync_at,
		        last_sync_error, created_at, updated_at
		 FROM git_repos WHERE team_id = $1 ORDER BY name`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing repos: %w", err)
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		var r Repo
		var lastSyncAt sql.NullTime
		var lastSyncSHA, lastSyncError sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.Branch, &r.Path,
			&r.PollInterval, &r.Credential, &r.AutoApply, &r.Prune, &r.Enabled,
			&lastSyncSHA, &lastSyncAt, &lastSyncError,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning repo: %w", err)
		}
		if lastSyncSHA.Valid {
			r.LastSyncSHA = lastSyncSHA.String
		}
		if lastSyncAt.Valid {
			t := lastSyncAt.Time
			r.LastSyncAt = &t
		}
		if lastSyncError.Valid {
			r.LastSyncError = lastSyncError.String
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// UpdateParams captures the mutable fields of a repo. Name and URL are
// intentionally omitted — changing either requires delete + recreate so
// that audit history clearly reflects the identity change.
type UpdateParams struct {
	Branch        string
	Path          string
	PollInterval  string
	Credential    string
	AutoApply     bool
	Prune         bool
	Enabled       bool
	WebhookSecret string // empty → NULL (no HMAC verification)
}

// Update replaces the mutable fields of a repo by name and emits a
// repo.updated audit event in the same transaction.
func (s *Store) Update(ctx context.Context, name string, p UpdateParams) (*Repo, error) {
	if err := ValidatePollInterval(p.PollInterval); err != nil {
		return nil, err
	}
	if p.Credential == "" {
		return nil, fmt.Errorf("credential is required")
	}

	teamID := auth.TeamIDFromContext(ctx)

	var updateWebhookSecret any
	if p.WebhookSecret != "" {
		updateWebhookSecret = p.WebhookSecret
	} else {
		updateWebhookSecret = nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var r Repo
	var updatedWebhookSecret sql.NullString
	err = tx.QueryRowContext(ctx,
		`UPDATE git_repos
		 SET branch = $3, path = $4, poll_interval = $5, credential = $6,
		     auto_apply = $7, prune = $8, enabled = $9, webhook_secret = $10,
		     updated_at = NOW()
		 WHERE name = $1 AND team_id = $2
		 RETURNING id, name, url, branch, path, poll_interval, credential,
		           auto_apply, prune, enabled, webhook_secret, created_at, updated_at`,
		name, teamID, p.Branch, p.Path, p.PollInterval, p.Credential,
		p.AutoApply, p.Prune, p.Enabled, updateWebhookSecret,
	).Scan(&r.ID, &r.Name, &r.URL, &r.Branch, &r.Path, &r.PollInterval,
		&r.Credential, &r.AutoApply, &r.Prune, &r.Enabled, &updatedWebhookSecret,
		&r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("updating repo %q: %w", name, err)
	}
	if updatedWebhookSecret.Valid {
		r.WebhookSecret = updatedWebhookSecret.String
	}

	if err := audit.EmitTx(ctx, tx, audit.Event{
		Timestamp: time.Now(),
		Actor:     s.actor(),
		Action:    audit.ActionRepoUpdated,
		Resource:  audit.Resource{Type: "git_repo", ID: r.ID},
		TeamID:    teamID,
		Metadata:  map[string]string{"name": name},
	}); err != nil {
		return nil, fmt.Errorf("emitting audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing repo update: %w", err)
	}
	return &r, nil
}

// Delete removes a repo by name and emits a repo.removed audit event in
// the same transaction. Returns ErrNotFound when no row matches.
//
// If cloneBasePath is non-empty, Delete also removes the cloned working
// tree at filepath.Join(cloneBasePath, repoID) after the DB transaction
// commits. Filesystem failures are logged but do not fail the call — the
// DB row is already gone, and an orphan directory is better than an orphan
// row.
func (s *Store) Delete(ctx context.Context, name, cloneBasePath string) error {
	teamID := auth.TeamIDFromContext(ctx)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var deletedID string
	err = tx.QueryRowContext(ctx,
		`DELETE FROM git_repos WHERE name = $1 AND team_id = $2 RETURNING id`,
		name, teamID,
	).Scan(&deletedID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return fmt.Errorf("deleting repo %q: %w", name, err)
	}

	if err := audit.EmitTx(ctx, tx, audit.Event{
		Timestamp: time.Now(),
		Actor:     s.actor(),
		Action:    audit.ActionRepoRemoved,
		Resource:  audit.Resource{Type: "git_repo", ID: deletedID},
		TeamID:    teamID,
		Metadata:  map[string]string{"name": name},
	}); err != nil {
		return fmt.Errorf("emitting audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing repo delete: %w", err)
	}

	if cloneBasePath != "" {
		cloneDir := filepath.Join(cloneBasePath, deletedID)
		if err := os.RemoveAll(cloneDir); err != nil {
			slog.Warn("repo delete: failed to remove cloned working tree",
				"clone_dir", cloneDir, "err", err)
		}
	}
	return nil
}

// UpdateSyncState records the outcome of a sync attempt. syncErr should
// be the empty string on success — it is stored as NULL so the
// LastSyncError field on Repo stays empty after a clean sync, even if
// an earlier attempt had failed.
func (s *Store) UpdateSyncState(ctx context.Context, id, sha, syncErr string) error {
	var errVal interface{}
	if syncErr == "" {
		errVal = nil
	} else {
		errVal = syncErr
	}
	_, err := s.DB.ExecContext(ctx,
		`UPDATE git_repos
		 SET last_sync_sha = $2, last_sync_at = NOW(), last_sync_error = $3
		 WHERE id = $1`,
		id, sha, errVal,
	)
	if err != nil {
		return fmt.Errorf("updating sync state for %s: %w", id, err)
	}
	return nil
}

// validateURL rejects URLs that embed credentials inline (the `user:pass@host`
// form). Operators must put credential material in a "git" secret and
// reference it via the Credential field, never inline in the URL, so we
// don't risk persisting or displaying tokens that live in git_repos.url.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url %q: %w", raw, err)
	}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			return fmt.Errorf("repo url must not embed credentials — use the --credential flag instead")
		}
	}
	return nil
}

// GetByID retrieves a repo by its UUID. Unlike Get, it does NOT filter
// by team_id — the webhook receiver at /hooks/git/<id> must be able to
// look up repos across any team because the URL carries the UUID and
// no auth context. Returns ErrNotFound when no row matches.
func (s *Store) GetByID(ctx context.Context, id string) (*Repo, error) {
	var r Repo
	var lastSyncAt sql.NullTime
	var lastSyncSHA, lastSyncError, webhookSecret sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, url, branch, path, poll_interval, credential,
		        auto_apply, prune, enabled, last_sync_sha, last_sync_at,
		        last_sync_error, webhook_secret, created_at, updated_at
		 FROM git_repos WHERE id = $1`,
		id,
	).Scan(&r.ID, &r.Name, &r.URL, &r.Branch, &r.Path, &r.PollInterval,
		&r.Credential, &r.AutoApply, &r.Prune, &r.Enabled,
		&lastSyncSHA, &lastSyncAt, &lastSyncError, &webhookSecret,
		&r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying repo by id: %w", err)
	}
	if lastSyncSHA.Valid {
		r.LastSyncSHA = lastSyncSHA.String
	}
	if lastSyncAt.Valid {
		t := lastSyncAt.Time
		r.LastSyncAt = &t
	}
	if lastSyncError.Valid {
		r.LastSyncError = lastSyncError.String
	}
	if webhookSecret.Valid {
		r.WebhookSecret = webhookSecret.String
	}
	return &r, nil
}

// Get retrieves a repo by name within the current team scope. Returns
// ErrNotFound when no row matches.
func (s *Store) Get(ctx context.Context, name string) (*Repo, error) {
	teamID := auth.TeamIDFromContext(ctx)

	var r Repo
	var lastSyncAt sql.NullTime
	var lastSyncSHA, lastSyncError, webhookSecret sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, url, branch, path, poll_interval, credential,
		        auto_apply, prune, enabled, last_sync_sha, last_sync_at,
		        last_sync_error, webhook_secret, created_at, updated_at
		 FROM git_repos WHERE name = $1 AND team_id = $2`,
		name, teamID,
	).Scan(&r.ID, &r.Name, &r.URL, &r.Branch, &r.Path, &r.PollInterval,
		&r.Credential, &r.AutoApply, &r.Prune, &r.Enabled,
		&lastSyncSHA, &lastSyncAt, &lastSyncError, &webhookSecret,
		&r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying repo: %w", err)
	}
	if lastSyncSHA.Valid {
		r.LastSyncSHA = lastSyncSHA.String
	}
	if lastSyncAt.Valid {
		t := lastSyncAt.Time
		r.LastSyncAt = &t
	}
	if lastSyncError.Valid {
		r.LastSyncError = lastSyncError.String
	}
	if webhookSecret.Valid {
		r.WebhookSecret = webhookSecret.String
	}
	return &r, nil
}
