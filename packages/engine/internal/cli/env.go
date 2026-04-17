package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/environment"
	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

// redactedPlaceholder is printed in place of env values in `env get` output
// unless --reveal is passed. Values files commonly contain secret-shaped
// material (API keys, connection strings, tokens) and must not leak to logs
// or terminal scrollback by default. Revealing is always audited.
const redactedPlaceholder = "<redacted>"

// newEnvCommand returns the "env" subcommand for managing named environments.
func newEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage named environments",
		Long: `Named environments (e.g., "production", "staging") store reusable input
and env variable sets for parameterized workflow runs. Combine with
` + "`mantle run --env <name>`" + ` and ` + "`mantle plan --env <name>`" + ` to promote the
same workflow across environments without rewriting CLI flags.

Override precedence (highest wins):
  --input flags > --values file > --env named environment > workflow defaults`,
	}

	cmd.AddCommand(newEnvCreateCommand())
	cmd.AddCommand(newEnvUpdateCommand())
	cmd.AddCommand(newEnvListCommand())
	cmd.AddCommand(newEnvGetCommand())
	cmd.AddCommand(newEnvDeleteCommand())

	return cmd
}

// newEnvCreateCommand returns the "env create" subcommand, which stores a new
// named environment loaded from a values file.
func newEnvCreateCommand() *cobra.Command {
	var fromFile string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a named environment",
		Long: `Creates a named environment from a YAML values file. The file must
contain top-level ` + "`inputs:`" + ` and/or ` + "`env:`" + ` keys. Names must match
[a-z0-9][a-z0-9_-]*. Fails if an environment with the same name already
exists in the team scope; use ` + "`mantle env update`" + ` to replace it.`,
		Example: `  mantle env create production --from prod.values.yaml
  mantle env create staging --from staging.values.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvWrite(cmd, args[0], fromFile, writeModeCreate)
		},
	}

	cmd.Flags().StringVar(&fromFile, "from", "", "Values file to load inputs and env from (required)")
	_ = cmd.MarkFlagRequired("from")

	return cmd
}

// newEnvUpdateCommand returns the "env update" subcommand, which replaces the
// stored inputs and env on an existing named environment.
func newEnvUpdateCommand() *cobra.Command {
	var fromFile string

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing named environment",
		Long: `Replaces the inputs and env of an existing environment with the contents
of a values file. Preserves the environment ID, creation timestamp, and audit
history. Fails if the environment does not exist.`,
		Example: `  mantle env update production --from prod.values.yaml`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvWrite(cmd, args[0], fromFile, writeModeUpdate)
		},
	}

	cmd.Flags().StringVar(&fromFile, "from", "", "Values file to load inputs and env from (required)")
	_ = cmd.MarkFlagRequired("from")

	return cmd
}

type envWriteMode int

const (
	writeModeCreate envWriteMode = iota
	writeModeUpdate
)

// runEnvWrite is the shared body for create/update: load a values file,
// dispatch to the matching Store method, and print a single-line confirmation.
func runEnvWrite(cmd *cobra.Command, name, fromFile string, mode envWriteMode) error {
	vals, err := workflow.LoadValues(fromFile)
	if err != nil {
		return fmt.Errorf("loading values file: %w", err)
	}

	store, cleanup, err := newEnvStore(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	var e *environment.Environment
	var verb string
	switch mode {
	case writeModeCreate:
		e, err = store.Create(cmd.Context(), name, vals.Inputs, vals.Env)
		verb = "Created"
	case writeModeUpdate:
		e, err = store.Update(cmd.Context(), name, vals.Inputs, vals.Env)
		verb = "Updated"
	default:
		return fmt.Errorf("unknown env write mode: %d", mode)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s environment %q\n", verb, e.Name)
	return nil
}

// newEnvListCommand returns the "env list" subcommand, which prints all named
// environments for the current team with their creation and update timestamps.
func newEnvListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all named environments",
		Long: `Lists every environment in the current team scope with creation and
update timestamps. Raw input and env values are never shown here — use
` + "`mantle env get <name> --reveal`" + ` to view them.`,
		Example: `  mantle env list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newEnvStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			envs, err := store.List(cmd.Context())
			if err != nil {
				return err
			}

			if len(envs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no environments)")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tCREATED\tUPDATED")
			for _, e := range envs {
				fmt.Fprintf(w, "%s\t%s\t%s\n",
					e.Name,
					e.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
					e.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
				)
			}
			return w.Flush()
		},
	}
}

// newEnvGetCommand returns the "env get" subcommand, which prints the stored
// inputs and env for a named environment. Env values are redacted unless the
// caller passes --reveal, which emits an environment.revealed audit event.
func newEnvGetCommand() *cobra.Command {
	var reveal bool

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show environment details",
		Long: `Shows the stored inputs and env for a named environment along with its
timestamps. Env values are redacted by default because values files often
contain secret-shaped material. Pass --reveal to print raw values; every
reveal emits an ` + "`environment.revealed`" + ` audit event.`,
		Example: `  mantle env get production
  mantle env get production --reveal`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newEnvStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			env, err := store.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if reveal {
				if revealErr := store.EmitReveal(cmd.Context(), env); revealErr != nil {
					return fmt.Errorf("recording reveal audit event: %w", revealErr)
				}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Name:       %s\n", env.Name)
			fmt.Fprintf(out, "ID:         %s\n", env.ID)
			fmt.Fprintf(out, "Created:    %s\n", env.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
			fmt.Fprintf(out, "Updated:    %s\n", env.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))

			if len(env.Inputs) > 0 {
				fmt.Fprintln(out, "\nInputs:")
				for _, k := range sortedKeys(env.Inputs) {
					fmt.Fprintf(out, "  %s: %v\n", k, env.Inputs[k])
				}
			}
			if len(env.Env) > 0 {
				fmt.Fprintln(out, "\nEnv:")
				for _, k := range sortedStringKeys(env.Env) {
					if reveal {
						fmt.Fprintf(out, "  %s: %s\n", k, env.Env[k])
					} else {
						fmt.Fprintf(out, "  %s: %s\n", k, redactedPlaceholder)
					}
				}
				if !reveal {
					fmt.Fprintln(out, "\n(Env values redacted. Pass --reveal to display; reveals are audited.)")
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&reveal, "reveal", false, "Print raw env values (emits an environment.revealed audit event)")
	return cmd
}

// newEnvDeleteCommand returns the "env delete" subcommand, which permanently
// removes a named environment. Requires --yes to confirm.
func newEnvDeleteCommand() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an environment",
		Long: `Permanently deletes a named environment. Requires --yes to confirm
because deletion cannot be undone from the CLI — referring workflow runs will
fail until the environment is recreated.`,
		Example: `  mantle env delete staging --yes`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing to delete %q without --yes", args[0])
			}

			store, cleanup, err := newEnvStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			if err := store.Delete(cmd.Context(), args[0]); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted environment %q\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm deletion (required)")
	return cmd
}

// newEnvStore builds an environment.Store from the current command context.
func newEnvStore(cmd *cobra.Command) (*environment.Store, func(), error) {
	cfg := config.FromContext(cmd.Context())
	if cfg == nil {
		return nil, nil, fmt.Errorf("config not loaded")
	}

	database, err := db.Open(cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &environment.Store{DB: database, Actor: "cli"}
	cleanup := func() { database.Close() }
	return store, cleanup, nil
}

// sortedKeys returns the keys of m in ascending order. Used for deterministic
// output in env get.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedStringKeys returns the keys of m in ascending order. Used for
// deterministic output in env get.
func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
