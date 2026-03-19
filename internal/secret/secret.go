package secret

import "time"

// Credential represents a stored credential with encrypted field data.
type Credential struct {
	ID        string
	Name      string
	Type      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
