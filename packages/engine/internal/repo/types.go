// Package repo manages registered GitOps source repositories (issue #16).
// Each Repo references encrypted auth material in the credentials table
// by name and stores last-sync state for observability. The sync engine
// itself (file discovery, validate/plan/apply pipeline) lives in a
// separate package and is out of scope for this package.
package repo

import (
	"fmt"
	"regexp"
	"time"
)

// maxRepoNameLength caps names at the DNS label limit (RFC 1035) — same
// rationale as environments: names embed into log lines, metric labels,
// and URL path segments without escaping.
const maxRepoNameLength = 63

// validRepoNamePattern enforces DNS-label-like names: lowercase
// alphanumerics, underscores, and hyphens, starting with an alphanumeric.
var validRepoNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Repo represents a registered git repository that Mantle pulls workflow
// definitions from.
type Repo struct {
	ID            string
	Name          string
	URL           string
	Branch        string
	Path          string
	PollInterval  string // Go duration literal, e.g., "60s"
	Credential    string // name of the git-type credential row
	AutoApply     bool
	Prune         bool
	Enabled       bool
	LastSyncSHA   string
	LastSyncAt    *time.Time
	LastSyncError string
	// WebhookSecret is the HMAC shared secret used to verify inbound push
	// webhooks from the git provider. SECURITY: this value is sensitive and
	// MUST NOT be printed to terminal or logs. The CLI intentionally never
	// renders it; any new caller must follow the same discipline.
	WebhookSecret string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ValidateName returns an error when name violates the allowed pattern
// or length. Exported because the CLI validates the flag before calling
// into the store for faster feedback on typos.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("repo name is required")
	}
	if len(name) > maxRepoNameLength {
		return fmt.Errorf("invalid repo name %q: length %d exceeds %d-char cap",
			name, len(name), maxRepoNameLength)
	}
	if !validRepoNamePattern.MatchString(name) {
		return fmt.Errorf("invalid repo name %q: must match %s",
			name, validRepoNamePattern.String())
	}
	return nil
}

// ValidatePollInterval returns an error when interval cannot be parsed
// as a Go duration or is below the 10-second minimum. The floor exists
// to prevent operators from hammering their git provider's rate limits.
func ValidatePollInterval(interval string) error {
	d, err := time.ParseDuration(interval)
	if err != nil {
		return fmt.Errorf("invalid poll_interval %q: %w", interval, err)
	}
	if d < 10*time.Second {
		return fmt.Errorf("poll_interval %q below 10s minimum", interval)
	}
	return nil
}
