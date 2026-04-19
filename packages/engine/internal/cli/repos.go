package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/repo"
	"github.com/dvflw/mantle/internal/repo/sync"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/spf13/cobra"
)

// newReposCommand returns the "repos" subcommand for managing registered
// GitOps source repositories (issue #16). Subcommands handle registration,
// listing, detailed status, and removal. Sync behavior lives in Plan B.
func newReposCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repos",
		Short: "Manage GitOps workflow source repositories",
		Long: `Registers GitOps source repositories whose workflow YAML definitions are
synced into this Mantle instance. This command manages the registry; use
` + "`mantle repos sync`" + ` for ad-hoc syncs, ` + "`mantle repos plan`" + ` to preview changes
without writing, ` + "`mantle repos apply`" + ` for manual control over auto_apply:false
repos, or start ` + "`mantle serve`" + ` to let the background poller run. Auth material
is stored in a "git" credential type (` + "`mantle secrets create --type git`" + `) and
referenced here by name.`,
	}
	cmd.AddCommand(newReposAddCommand())
	cmd.AddCommand(newReposUpdateCommand())
	cmd.AddCommand(newReposListCommand())
	cmd.AddCommand(newReposStatusCommand())
	cmd.AddCommand(newReposRemoveCommand())
	cmd.AddCommand(newReposSyncCommand())
	cmd.AddCommand(newReposPlanCommand())
	cmd.AddCommand(newReposApplyCommand())
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
	var url, branch, path, pollInterval, credential, webhookSecret string
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
				Name:          args[0],
				URL:           url,
				Branch:        branch,
				Path:          path,
				PollInterval:  pollInterval,
				Credential:    credential,
				AutoApply:     autoApply,
				Prune:         prune,
				WebhookSecret: webhookSecret,
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
	cmd.Flags().BoolVar(&prune, "prune", true, "When true, workflows deleted from the repo are disabled in Mantle")
	cmd.Flags().StringVar(&webhookSecret, "webhook-secret", "",
		"HMAC secret for push webhook verification (empty = no verification)")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("credential")

	return cmd
}

func newReposStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show detailed status for a registered repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			r, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Name:         %s\n", r.Name)
			fmt.Fprintf(out, "ID:           %s\n", r.ID)
			fmt.Fprintf(out, "URL:          %s\n", r.URL)
			fmt.Fprintf(out, "Branch:       %s\n", r.Branch)
			fmt.Fprintf(out, "Path:         %s\n", r.Path)
			fmt.Fprintf(out, "Poll:         %s\n", r.PollInterval)
			fmt.Fprintf(out, "Credential:   %s\n", r.Credential)
			if r.AutoApply {
				fmt.Fprintln(out, "Auto-Apply:   true")
			} else {
				fmt.Fprintln(out, "Auto-Apply:   false (manual sync only — background poller does not run this repo)")
			}
			fmt.Fprintf(out, "Prune:        %t\n", r.Prune)
			fmt.Fprintf(out, "Enabled:      %t\n", r.Enabled)
			if r.LastSyncAt != nil {
				fmt.Fprintf(out, "Last Sync:    %s (SHA %s)\n",
					r.LastSyncAt.UTC().Format("2006-01-02 15:04:05 UTC"), r.LastSyncSHA)
			} else {
				fmt.Fprintln(out, "Last Sync:    (never)")
			}
			if r.LastSyncError != "" {
				fmt.Fprintf(out, "Last Error:   %s\n", r.LastSyncError)
			}
			return nil
		},
	}
}

func newReposRemoveCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Unregister a GitOps source repo",
		Long: `Unregisters a repo. Any previously applied workflows remain in place — this
command only stops future syncs. Requires --yes to confirm.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing to remove %q without --yes", args[0])
			}
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			cfg := config.FromContext(cmd.Context())
			cloneBase := ""
			if cfg != nil {
				artifactBase := cfg.Storage.Path
				if artifactBase == "" {
					artifactBase = filepath.Join(os.TempDir(), "mantle-artifacts")
				}
				cloneBase = filepath.Join(artifactBase, "git")
			}
			if err := store.Delete(cmd.Context(), args[0], cloneBase); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed repo %q\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm deletion (required)")
	return cmd
}

func newReposListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered GitOps repos",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			repos, err := store.List(cmd.Context())
			if err != nil {
				return err
			}
			if len(repos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no repos)")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tURL\tBRANCH\tAUTO-APPLY\tENABLED\tLAST SYNC")
			for _, r := range repos {
				last := "(never)"
				if r.LastSyncAt != nil {
					last = r.LastSyncAt.UTC().Format("2006-01-02 15:04:05 UTC")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%t\t%s\n",
					r.Name, r.URL, r.Branch, r.AutoApply, r.Enabled, last)
			}
			return w.Flush()
		},
	}
}

func newReposUpdateCommand() *cobra.Command {
	var branch, path, pollInterval, credential, webhookSecret string
	var autoApply, prune, enabled bool

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a registered repo's mutable fields",
		Long: `Replaces the branch, path, poll-interval, credential, auto-apply, prune,
and enabled flags on an existing repo. URL and name are immutable — to
change them, delete and recreate.
All flags must be provided — update replaces the full set of mutable fields.
Omitting a flag resets that field to its default value.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			r, err := store.Update(cmd.Context(), args[0], repo.UpdateParams{
				Branch: branch, Path: path, PollInterval: pollInterval,
				Credential: credential, AutoApply: autoApply, Prune: prune, Enabled: enabled,
				WebhookSecret: webhookSecret,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated repo %q\n", r.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "main", "Branch to sync")
	cmd.Flags().StringVar(&path, "path", "/", "Subdirectory inside the repo to scan")
	cmd.Flags().StringVar(&pollInterval, "poll-interval", "60s", "Interval between syncs")
	cmd.Flags().StringVar(&credential, "credential", "", "Git credential name (required)")
	cmd.Flags().BoolVar(&autoApply, "auto-apply", true, "Automatically apply changes")
	cmd.Flags().BoolVar(&prune, "prune", true, "Disable workflows deleted from the repo")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Whether the repo is active")
	cmd.Flags().StringVar(&webhookSecret, "webhook-secret", "",
		"HMAC secret for push webhook verification (empty = no verification)")
	_ = cmd.MarkFlagRequired("credential")
	return cmd
}

func newReposSyncCommand() *cobra.Command {
	var fromDir string
	cmd := &cobra.Command{
		Use:   "sync <name>",
		Short: "Force an immediate sync of a registered repo",
		Long: `Runs one sync pass for the named repo: pulls the remote, walks the
configured path, and applies each workflow YAML. Returns after the sync
completes. Use --from-dir to point at a pre-populated directory instead
of cloning (useful for tests and CI-driven deployments).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			r, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			driver := buildSyncDriver(cmd, store, fromDir)
			report, err := sync.SyncRepo(cmd.Context(), store.DB, store, r, driver)
			if err != nil {
				return fmt.Errorf("sync %q: %w", r.Name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Synced %s — applied=%d unchanged=%d failures=%d (SHA %s)\n",
				r.Name, report.Applied, report.Unchanged, len(report.Failures), report.SHA)
			if len(report.Failures) > 0 {
				for _, f := range report.Failures {
					fmt.Fprintf(cmd.ErrOrStderr(), "  FAIL %s: %s\n", f.RelPath, f.Err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromDir, "from-dir", "", "Path to an already-cloned repo directory (skips git pull; useful for CI pipelines or local testing)")
	return cmd
}

func newReposPlanCommand() *cobra.Command {
	var fromDir string
	cmd := &cobra.Command{
		Use:   "plan <name>",
		Short: "Dry-run: show what a sync would change",
		Long: `Plans a sync without writing anything: pulls the repo, discovers files,
and classifies each as "would apply" (new or changed) or "unchanged". Use
before mantle repos apply to preview pending changes before they land.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			r, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			driver := buildSyncDriver(cmd, store, fromDir)
			report, err := sync.PlanRepo(cmd.Context(), store.DB, r, driver)
			if err != nil {
				return fmt.Errorf("plan %q: %w", r.Name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Plan for %s — would apply=%d unchanged=%d failures=%d (SHA %s)\n",
				r.Name, report.Applied, report.Unchanged, len(report.Failures), report.SHA)
			for _, f := range report.Failures {
				fmt.Fprintf(cmd.ErrOrStderr(), "  FAIL %s: %s\n", f.RelPath, f.Err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromDir, "from-dir", "", "Path to an already-cloned repo directory (skips git pull; useful for CI pipelines or local testing)")
	return cmd
}

func newReposApplyCommand() *cobra.Command {
	var fromDir string
	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Apply pending workflow changes from a repo",
		Long: `Runs one sync pass for the named repo. Useful when auto_apply is false and you
want explicit control over when workflow YAML lands in Mantle. Also works for
auto_apply:true repos when you want to apply immediately without waiting for
the next poll.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newRepoStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			r, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			driver := buildSyncDriver(cmd, store, fromDir)
			report, err := sync.SyncRepo(cmd.Context(), store.DB, store, r, driver)
			if err != nil {
				return fmt.Errorf("apply %q: %w", r.Name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Applied %s — applied=%d unchanged=%d failures=%d (SHA %s)\n",
				r.Name, report.Applied, report.Unchanged, len(report.Failures), report.SHA)
			for _, f := range report.Failures {
				fmt.Fprintf(cmd.ErrOrStderr(), "  FAIL %s: %s\n", f.RelPath, f.Err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromDir, "from-dir", "", "Path to an already-cloned repo directory (skips git pull; useful for CI pipelines or local testing)")
	return cmd
}

// buildSyncDriver picks NoopDriver when --from-dir is set, otherwise a
// GoGitDriver rooted at a temp path. Extracted from newReposSyncCommand so
// plan and apply don't duplicate the branching logic.
func buildSyncDriver(cmd *cobra.Command, store *repo.Store, fromDir string) sync.Driver {
	if fromDir != "" {
		return &sync.NoopDriver{BasePath: fromDir}
	}
	var secretResolver *secret.Resolver
	if cfg := config.FromContext(cmd.Context()); cfg != nil && cfg.Encryption.Key != "" {
		encryptor, encErr := secret.NewEncryptor(cfg.Encryption.Key)
		if encErr == nil {
			secretResolver = &secret.Resolver{
				Store: &secret.Store{DB: store.DB, Encryptor: encryptor},
			}
		}
	}
	return &sync.GoGitDriver{
		BasePath: filepath.Join(os.TempDir(), "mantle-repos"),
		Auth:     sync.NewAuthResolver(cmd.Context(), secretResolver),
	}
}
