package sync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dvflw/mantle/internal/repo"
)

// setupBareRepo creates a bare git repo on disk, seeds one commit that
// contains workflows/wf.yaml, and returns (bareRepoPath, commitSHA).
// Uses the host's git CLI so the test doesn't depend on go-git's own
// bare-repo construction paths being right.
func setupBareRepo(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git CLI not available")
	}
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed")
	bare := filepath.Join(dir, "bare.git")
	mustRun(t, dir, "git", "init", "-b", "main", seed)
	_ = os.MkdirAll(filepath.Join(seed, "workflows"), 0o755)
	if err := os.WriteFile(filepath.Join(seed, "workflows", "wf.yaml"), []byte("name: wf\n"), 0o644); err != nil {
		t.Fatalf("write wf: %v", err)
	}
	mustRun(t, seed, "git", "-c", "user.email=t@t", "-c", "user.name=t", "add", ".")
	mustRun(t, seed, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "seed")
	mustRun(t, dir, "git", "clone", "--bare", seed, bare)
	shaBytes, err := exec.Command("git", "-C", bare, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	sha := string(shaBytes)
	sha = sha[:len(sha)-1] // trim newline
	return bare, sha
}

func mustRun(t *testing.T, dir, bin string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", bin, args, err, out)
	}
}

func TestGoGitDriver_Pull_ClonesThenUpdates(t *testing.T) {
	bare, initialSHA := setupBareRepo(t)
	base := t.TempDir()

	d := &GoGitDriver{BasePath: base}
	r := &repo.Repo{ID: "r1", URL: bare, Branch: "main"}

	// First pull: fresh clone.
	got, err := d.Pull(context.Background(), r)
	if err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if got.SHA != initialSHA {
		t.Errorf("SHA: got %q, want %q", got.SHA, initialSHA)
	}
	if _, err := os.Stat(filepath.Join(got.LocalPath, "workflows", "wf.yaml")); err != nil {
		t.Errorf("wf.yaml not checked out: %v", err)
	}

	// Add a new commit to the source, push to bare, pull again.
	seed := filepath.Join(filepath.Dir(bare), "seed")
	if err := os.WriteFile(filepath.Join(seed, "workflows", "wf2.yaml"), []byte("name: wf2\n"), 0o644); err != nil {
		t.Fatalf("write wf2: %v", err)
	}
	mustRun(t, seed, "git", "-c", "user.email=t@t", "-c", "user.name=t", "add", ".")
	mustRun(t, seed, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "two")
	mustRun(t, seed, "git", "push", bare, "main")

	got2, err := d.Pull(context.Background(), r)
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if got2.SHA == initialSHA {
		t.Error("SHA did not advance after second commit")
	}
	if _, err := os.Stat(filepath.Join(got2.LocalPath, "workflows", "wf2.yaml")); err != nil {
		t.Errorf("wf2.yaml not checked out after update: %v", err)
	}
}
