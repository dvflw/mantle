package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/engine"
	"github.com/spf13/cobra"
)

func newCleanupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove old execution data based on retention policy",
		Long: `Deletes workflow executions and audit events older than the specified
retention period. Uses --execution-days and --audit-days flags, falling back
to the retention config in mantle.yaml or environment variables.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			execDays, _ := cmd.Flags().GetInt("execution-days")
			auditDays, _ := cmd.Flags().GetInt("audit-days")

			// Fall back to config values if flags were not explicitly set.
			if !cmd.Flags().Changed("execution-days") {
				execDays = cfg.Retention.ExecutionDays
			}
			if !cmd.Flags().Changed("audit-days") {
				auditDays = cfg.Retention.AuditDays
			}

			if execDays <= 0 && auditDays <= 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No retention period configured. Use --execution-days or --audit-days, or set retention config.")
				return nil
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			ctx := cmd.Context()

			if execDays > 0 {
				deleted, err := engine.CleanupExecutions(ctx, database, execDays)
				if err != nil {
					return fmt.Errorf("cleaning executions: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted %d workflow execution(s) older than %d day(s).\n", deleted, execDays)
			}

			if auditDays > 0 {
				deleted, err := engine.CleanupAuditEvents(ctx, database, auditDays)
				if err != nil {
					return fmt.Errorf("cleaning audit events: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted %d audit event(s) older than %d day(s).\n", deleted, auditDays)
			}

			return nil
		},
	}

	cmd.Flags().Int("execution-days", 0, "Delete executions older than N days (0 = use config)")
	cmd.Flags().Int("audit-days", 0, "Delete audit events older than N days (0 = use config)")

	return cmd
}
