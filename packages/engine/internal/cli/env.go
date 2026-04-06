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

// newEnvCommand returns the "env" subcommand for managing named environments.
func newEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage named environments",
		Long:  "Create, list, show, and delete named environments for parameterized workflow execution.",
	}

	cmd.AddCommand(newEnvCreateCommand())
	cmd.AddCommand(newEnvListCommand())
	cmd.AddCommand(newEnvShowCommand())
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
		Long:  "Creates a named environment from a values file.",
		Example: `  mantle env create production --from prod.values.yaml
  mantle env create staging --from staging.values.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			vals, err := workflow.LoadValues(fromFile)
			if err != nil {
				return fmt.Errorf("loading values file: %w", err)
			}

			store, cleanup, err := newEnvStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			env, err := store.Create(cmd.Context(), name, vals.Inputs, vals.Env)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created environment %q\n", env.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&fromFile, "from", "", "Values file to load inputs and env from (required)")
	_ = cmd.MarkFlagRequired("from")

	return cmd
}

// newEnvListCommand returns the "env list" subcommand, which prints all named
// environments for the current team.
func newEnvListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all environments",
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
			fmt.Fprintln(w, "NAME\tCREATED")
			for _, e := range envs {
				fmt.Fprintf(w, "%s\t%s\n", e.Name, e.CreatedAt.Format("2006-01-02 15:04:05"))
			}
			return w.Flush()
		},
	}
}

// newEnvShowCommand returns the "env show" subcommand, which prints the inputs
// and env vars stored in a named environment.
func newEnvShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show environment details",
		Args:  cobra.ExactArgs(1),
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

			fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", env.Name)
			if len(env.Inputs) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nInputs:")
				keys := make([]string, 0, len(env.Inputs))
				for k := range env.Inputs {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %v\n", k, env.Inputs[k])
				}
			}
			if len(env.Env) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nEnv:")
				keys := make([]string, 0, len(env.Env))
				for k := range env.Env {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", k, env.Env[k])
				}
			}
			return nil
		},
	}
}

// newEnvDeleteCommand returns the "env delete" subcommand, which removes a
// named environment from the database.
func newEnvDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

	store := &environment.Store{DB: database}
	cleanup := func() { database.Close() }
	return store, cleanup, nil
}
