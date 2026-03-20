package auth

import (
	"context"
	"database/sql"
	"fmt"
)

// Store handles CRUD for teams, users, and API keys.
type Store struct {
	DB *sql.DB
}

// --- Teams ---

func (s *Store) CreateTeam(ctx context.Context, name string) (*Team, error) {
	var t Team
	err := s.DB.QueryRowContext(ctx,
		`INSERT INTO teams (name) VALUES ($1) RETURNING id, name, created_at, updated_at`,
		name,
	).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating team: %w", err)
	}
	return &t, nil
}

func (s *Store) ListTeams(ctx context.Context) ([]Team, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, created_at, updated_at FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

func (s *Store) DeleteTeam(ctx context.Context, name string) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM teams WHERE name = $1 AND id != $2`, name, DefaultTeamID)
	if err != nil {
		return fmt.Errorf("deleting team: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("team %q not found or cannot delete default team", name)
	}
	return nil
}

func (s *Store) GetTeamByName(ctx context.Context, name string) (*Team, error) {
	var t Team
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, name, created_at, updated_at FROM teams WHERE name = $1`, name,
	).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("team %q not found", name)
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// --- Users ---

func (s *Store) CreateUser(ctx context.Context, email, name, teamID string, role Role) (*User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx,
		`INSERT INTO users (email, name, team_id, role) VALUES ($1, $2, $3, $4)
		 RETURNING id, email, name, team_id, role, created_at, updated_at`,
		email, name, teamID, string(role),
	).Scan(&u.ID, &u.Email, &u.Name, &u.TeamID, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return &u, nil
}

func (s *Store) ListUsers(ctx context.Context, teamID string) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, email, name, team_id, role, created_at, updated_at FROM users WHERE team_id = $1 ORDER BY name`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.TeamID, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) DeleteUser(ctx context.Context, email string) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE email = $1`, email)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user %q not found", email)
	}
	return nil
}

func (s *Store) SetUserRole(ctx context.Context, email string, role Role) error {
	result, err := s.DB.ExecContext(ctx,
		`UPDATE users SET role = $1, updated_at = NOW() WHERE email = $2`,
		string(role), email,
	)
	if err != nil {
		return fmt.Errorf("updating role: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user %q not found", email)
	}
	return nil
}

// LookupUserByEmail finds a user by their email address.
// Returns nil, nil if no user is found.
func (s *Store) LookupUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, email, name, team_id, role, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.TeamID, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// --- API Keys ---

func (s *Store) CreateAPIKey(ctx context.Context, userID, name string) (rawKey string, key *APIKey, err error) {
	raw, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		return "", nil, err
	}

	var k APIKey
	err = s.DB.QueryRowContext(ctx,
		`INSERT INTO api_keys (user_id, name, key_hash, key_prefix) VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, name, key_hash, key_prefix, created_at`,
		userID, name, hash, prefix,
	).Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.CreatedAt)
	if err != nil {
		return "", nil, fmt.Errorf("storing API key: %w", err)
	}

	return raw, &k, nil
}

// LookupAPIKey finds the user associated with a raw API key.
func (s *Store) LookupAPIKey(ctx context.Context, rawKey string) (*User, error) {
	hash := HashAPIKey(rawKey)

	var u User
	err := s.DB.QueryRowContext(ctx,
		`SELECT u.id, u.email, u.name, u.team_id, u.role, u.created_at, u.updated_at
		 FROM api_keys k JOIN users u ON k.user_id = u.id
		 WHERE k.key_hash = $1`, hash,
	).Scan(&u.ID, &u.Email, &u.Name, &u.TeamID, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Update last_used_at.
	s.DB.ExecContext(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE key_hash = $1`, hash)

	return &u, nil
}
