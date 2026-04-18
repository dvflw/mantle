// Package sync implements the GitOps sync engine for issue #16. It reads
// workflow YAML files from a path a Driver populates, diffs them against
// the currently applied workflow versions, and calls workflow.Save for
// anything that changed. The engine does not care how the Driver
// populated the path — that separation lets us ship a go-git driver for
// standalone deployments, a noop driver for tests and CI-driven
// deployments, and (in a future release) a k8s sidecar driver that
// reads from a shared volume.
package sync

import (
	"context"

	"github.com/dvflw/mantle/internal/repo"
)

// PullResult describes the state of a repo after a Driver pull.
type PullResult struct {
	LocalPath string // absolute path where the repo was checked out
	SHA       string // commit SHA at HEAD after the pull
}

// Driver is the interface sync drivers implement. Pull must either leave
// a fully-checked-out working tree at LocalPath pointing at the tip of
// the configured branch, or return an error. The sync engine owns the
// lifecycle of LocalPath — drivers should place it inside the artifact
// store's git/ prefix so it survives across syncs but is reapable.
type Driver interface {
	Pull(ctx context.Context, r *repo.Repo) (PullResult, error)
}
