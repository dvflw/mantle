package cli

import (
	"fmt"
	"os"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

func newApplyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <file>",
		Short: "Apply a workflow definition",
		Long:  "Validates and stores a workflow definition as a new immutable version in the database.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			rawContent, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Errorf("reading %s: %w", filename, err)
			}

			result, err := workflow.ParseBytes(rawContent)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", filename, err)
			}

			version, err := workflow.Save(cmd.Context(), database, result, rawContent)
			if err != nil {
				return err
			}

			if version == 0 {
				latestVersion, _ := workflow.GetLatestVersion(cmd.Context(), database, result.Workflow.Name)
				fmt.Fprintf(cmd.OutOrStdout(), "No changes — %s is already at version %d\n",
					result.Workflow.Name, latestVersion)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Applied %s version %d\n",
					result.Workflow.Name, version)
			}

			return nil
		},
	}
}
