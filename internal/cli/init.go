package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize Mantle — run database migrations",
		Long:  "Runs all pending database migrations to set up or upgrade the Mantle schema.",
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

			fmt.Fprintln(cmd.OutOrStdout(), "Running migrations...")
			if err := db.Migrate(cmd.Context(), database); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Migrations complete.")
			return nil
		},
	}
}
