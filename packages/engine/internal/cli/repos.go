package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/repo"
	"github.com/spf13/cobra"
)

// newReposCommand returns the "repos" subcommand for managing registered
// GitOps source repositories (issue #16). Subcommands handle registration,
// listing, detailed status, and removal. Sync behavior lives in Plan B.
func newReposCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repos",
		Short: "Manage GitOps workflow source repositories",
		Long: `Registered repos are periodically pulled by the git-sync sidecar and
their .yaml workflow definitions are applied to this Mantle instance.

Auth material is stored in a "git" credential type (` + "`mantle secrets create --type git`" + `)
and referenced here by name.`,
	}
	return cmd
}

// newRepoStore builds a repo.Store from the current command context.
func newRepoStore(cmd *cobra.Command) (*repo.Store, func(), error) {
	cfg := config.FromContext(cmd.Context())
	if cfg == nil {
		return nil, nil, fmt.Errorf("config not loaded")
	}
	database, err := db.Open(cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	store := &repo.Store{DB: database, Actor: "cli"}
	cleanup := func() { database.Close() }
	return store, cleanup, nil
}
