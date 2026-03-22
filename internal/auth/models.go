package auth

import "time"

// Role represents a user's permission level.
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleTeamOwner Role = "team_owner"
	RoleOperator  Role = "operator"
)

// Team represents a team/organization.
type Team struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// User represents a user within a team.
type User struct {
	ID        string
	Email     string
	Name      string
	TeamID    string
	Role      Role
	CreatedAt time.Time
	UpdatedAt time.Time
}

// APIKey represents a hashed API key record.
type APIKey struct {
	ID         string
	UserID     string
	Name       string
	KeyHash    string
	KeyPrefix  string
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// DefaultTeamID is the ID of the auto-created default team for single-tenant migration.
const DefaultTeamID = "00000000-0000-0000-0000-000000000001"
