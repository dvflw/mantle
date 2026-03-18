package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run pending database migrations",
		Long:  "Runs all pending database migrations to upgrade the Mantle schema.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			if err := db.Migrate(cmd.Context(), database); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Migrations complete.")
			return nil
		},
	}

	cmd.AddCommand(newMigrateStatusCommand())
	cmd.AddCommand(newMigrateDownCommand())

	return cmd
}

func newMigrateStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		Long:  "Shows which migrations have been applied and which are pending.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			status, err := db.MigrateStatus(cmd.Context(), database)
			if err != nil {
				return fmt.Errorf("failed to get migration status: %w", err)
			}

			fmt.Fprint(cmd.OutOrStdout(), status)
			return nil
		},
	}
}

func newMigrateDownCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Roll back the last migration",
		Long:  "Rolls back the most recently applied database migration.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			if err := db.MigrateDown(cmd.Context(), database); err != nil {
				return fmt.Errorf("rollback failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Rollback complete.")
			return nil
		},
	}
}
