package cli

import (
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status <execution-id>",
		Short: "View execution state",
		Long:  "Shows the current state of a workflow execution.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			execID := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			var workflowName string
			var version int
			var status string
			var startedAt, completedAt *time.Time
			err = database.QueryRowContext(cmd.Context(),
				`SELECT workflow_name, workflow_version, status, started_at, completed_at
				 FROM workflow_executions WHERE id = $1`, execID,
			).Scan(&workflowName, &version, &status, &startedAt, &completedAt)
			if err != nil {
				return fmt.Errorf("execution %q not found", execID)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Execution: %s\n", execID)
			fmt.Fprintf(cmd.OutOrStdout(), "Workflow:  %s (version %d)\n", workflowName, version)
			fmt.Fprintf(cmd.OutOrStdout(), "Status:    %s\n", status)
			if startedAt != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Started:   %s\n", startedAt.Format(time.RFC3339))
			}
			if completedAt != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Completed: %s\n", completedAt.Format(time.RFC3339))
			}

			// Step summary.
			rows, err := database.QueryContext(cmd.Context(),
				`SELECT status, COUNT(*) FROM step_executions
				 WHERE execution_id = $1 GROUP BY status`, execID,
			)
			if err != nil {
				return fmt.Errorf("querying step counts: %w", err)
			}
			defer rows.Close()

			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Steps:")
			for rows.Next() {
				var stepStatus string
				var count int
				if err := rows.Scan(&stepStatus, &count); err != nil {
					return fmt.Errorf("scanning: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %d\n", stepStatus, count)
			}

			return rows.Err()
		},
	}
}
