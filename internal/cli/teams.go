package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newTeamsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teams",
		Short: "Manage teams",
		Long:  "Create, list, and delete teams.",
	}

	cmd.AddCommand(newTeamsCreateCommand())
	cmd.AddCommand(newTeamsListCommand())
	cmd.AddCommand(newTeamsDeleteCommand())

	return cmd
}

func newTeamsCreateCommand() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new team",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newAuthStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			team, err := store.CreateTeam(cmd.Context(), name)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created team %s (id: %s)\n", team.Name, team.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "team name (required)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newTeamsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all teams",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newAuthStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			teams, err := store.ListTeams(cmd.Context())
			if err != nil {
				return err
			}

			if len(teams) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no teams)")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tCREATED")
			for _, t := range teams {
				fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, t.ID, t.CreatedAt.Format("2006-01-02 15:04:05"))
			}
			return w.Flush()
		},
	}
}

func newTeamsDeleteCommand() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a team",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newAuthStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			if err := store.DeleteTeam(cmd.Context(), name); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted team %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "team name to delete (required)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// newAuthStore builds an auth.Store from the current command context.
func newAuthStore(cmd *cobra.Command) (*auth.Store, func(), error) {
	cfg := config.FromContext(cmd.Context())
	if cfg == nil {
		return nil, nil, fmt.Errorf("config not loaded")
	}

	database, err := db.Open(cfg.Database.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &auth.Store{DB: database}
	cleanup := func() { database.Close() }
	return store, cleanup, nil
}
