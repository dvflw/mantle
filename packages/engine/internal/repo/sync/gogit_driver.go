package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvflw/mantle/internal/repo"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// GoGitDriver clones or fast-forwards the configured branch into a
// subdirectory of BasePath. Intended for standalone deployments where
// Mantle is the git client — k8s deployments should run git-sync as a
// sidecar and use NoopDriver pointing at the shared volume instead.
type GoGitDriver struct {
	// BasePath is the parent directory for every cloned repo. Each
	// repo.Repo gets a subdir named after its UUID so names can never
	// collide and the path is stable across restarts.
	BasePath string
	// Auth is an optional hook that resolves the repo's credential name
	// to go-git auth material (e.g., an HTTPS token or an SSH key). The
	// sync engine's wiring populates this; tests leave it nil to exercise
	// the anonymous-clone path against local bare repos.
	Auth func(credentialName string) (transport.AuthMethod, error)
}

// Pull clones if the directory does not exist, otherwise fetches and
// hard-resets to the tip of the configured branch. Returns the HEAD SHA
// after the operation.
func (d *GoGitDriver) Pull(ctx context.Context, r *repo.Repo) (PullResult, error) {
	if d.BasePath == "" {
		return PullResult{}, fmt.Errorf("GoGitDriver requires BasePath")
	}
	if r == nil || r.ID == "" || r.URL == "" {
		return PullResult{}, fmt.Errorf("GoGitDriver requires repo with ID and URL")
	}
	dir := filepath.Join(d.BasePath, r.ID)

	var auth transport.AuthMethod
	if d.Auth != nil && r.Credential != "" {
		resolved, err := d.Auth(r.Credential)
		if err != nil {
			return PullResult{}, fmt.Errorf("resolving credential %q: %w", r.Credential, err)
		}
		auth = resolved
	}

	branch := r.Branch
	if branch == "" {
		branch = "main"
	}

	repository, err := git.PlainOpen(dir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repository, err = git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
			URL:           r.URL,
			Auth:          auth,
			ReferenceName: plumbing.NewBranchReferenceName(branch),
			SingleBranch:  true,
			Depth:         1,
		})
		if err != nil {
			// Clean up a partial clone so the next attempt starts fresh.
			_ = os.RemoveAll(dir)
			return PullResult{}, fmt.Errorf("clone: %w", err)
		}
	} else if err != nil {
		return PullResult{}, fmt.Errorf("open existing repo: %w", err)
	} else {
		if fetchErr := repository.FetchContext(ctx, &git.FetchOptions{
			Auth: auth,
			RefSpecs: []config.RefSpec{
				config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)),
			},
			Force: true,
		}); fetchErr != nil && !errors.Is(fetchErr, git.NoErrAlreadyUpToDate) {
			return PullResult{}, fmt.Errorf("fetch: %w", fetchErr)
		}
		wt, wtErr := repository.Worktree()
		if wtErr != nil {
			return PullResult{}, fmt.Errorf("worktree: %w", wtErr)
		}
		remoteRef, refErr := repository.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
		if refErr != nil {
			return PullResult{}, fmt.Errorf("resolve remote branch %q: %w", branch, refErr)
		}
		if resetErr := wt.Reset(&git.ResetOptions{Commit: remoteRef.Hash(), Mode: git.HardReset}); resetErr != nil {
			return PullResult{}, fmt.Errorf("hard reset to %s: %w", remoteRef.Hash(), resetErr)
		}
	}

	head, err := repository.Head()
	if err != nil {
		return PullResult{}, fmt.Errorf("resolving HEAD: %w", err)
	}
	return PullResult{LocalPath: dir, SHA: head.Hash().String()}, nil
}
