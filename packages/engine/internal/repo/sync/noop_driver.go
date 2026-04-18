package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvflw/mantle/internal/repo"
)

// NoopDriver returns a path under BasePath without fetching anything.
// Used by tests (callers pre-populate the directory with YAML fixtures)
// and by CI-driven deployments where external tooling already checks
// out the repo before Mantle reads from it.
type NoopDriver struct {
	BasePath string // directory containing one subdir per repo ID
	SHA      string // fake SHA returned by Pull, lets tests assert it propagates
}

// Pull returns the deterministic path BasePath/<repo.ID> and creates
// the directory on demand so downstream callers don't fail on os.Stat.
func (d *NoopDriver) Pull(_ context.Context, r *repo.Repo) (PullResult, error) {
	if d.BasePath == "" {
		return PullResult{}, fmt.Errorf("NoopDriver requires BasePath")
	}
	if r == nil || r.ID == "" {
		return PullResult{}, fmt.Errorf("NoopDriver requires a repo with an ID")
	}
	path := filepath.Join(d.BasePath, r.ID)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return PullResult{}, fmt.Errorf("creating %s: %w", path, err)
	}
	return PullResult{LocalPath: path, SHA: d.SHA}, nil
}
