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
	cmd.AddCommand(newReposAddCommand())
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

func newReposAddCommand() *cobra.Command {
	var url, branch, path, pollInterval, credential string
	var autoApply, prune bool

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new GitOps source repo",
		Long: `Registers a new repository to sync workflow definitions from. The named
credential must already exist and be of type "git".`,
		Example: `  mantle repos add acme --url https://github.com/acme/workflows.git --credential github-pat
  mantle repos add staging --url git@github.com:acme/wf.git --credential github-ssh --branch release`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			r, err := store.Create(cmd.Context(), repo.CreateParams{
				Name:         args[0],
				URL:          url,
				Branch:       branch,
				Path:         path,
				PollInterval: pollInterval,
				Credential:   credential,
				AutoApply:    autoApply,
				Prune:        prune,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added repo %q (%s)\n", r.Name, r.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "Git repository URL (required)")
	cmd.Flags().StringVar(&branch, "branch", "main", "Branch to sync")
	cmd.Flags().StringVar(&path, "path", "/", "Subdirectory inside the repo to scan")
	cmd.Flags().StringVar(&pollInterval, "poll-interval", "60s", "Interval between syncs (Go duration, min 10s)")
	cmd.Flags().StringVar(&credential, "credential", "", "Git credential name (required)")
	cmd.Flags().BoolVar(&autoApply, "auto-apply", true, "Automatically apply changes (false = plan-only)")
	cmd.Flags().BoolVar(&prune, "prune", true, "Disable workflows removed from the repo")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("credential")

	return cmd
}
