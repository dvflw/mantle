package cli

import (
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <execution-id>",
		Short: "View execution logs",
		Long:  "Shows step-by-step execution history with timing, status, and outputs.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			execID := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			// Fetch execution info.
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
				if startedAt != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Duration:  %s\n", completedAt.Sub(*startedAt).Round(time.Millisecond))
				}
			}
			fmt.Fprintln(cmd.OutOrStdout())

			// Fetch step executions.
			rows, err := database.QueryContext(cmd.Context(),
				`SELECT step_name, status, error, started_at, completed_at
				 FROM step_executions WHERE execution_id = $1
				 ORDER BY created_at ASC`, execID,
			)
			if err != nil {
				return fmt.Errorf("querying steps: %w", err)
			}
			defer rows.Close()

			fmt.Fprintln(cmd.OutOrStdout(), "Steps:")
			for rows.Next() {
				var stepName, stepStatus string
				var stepError *string
				var stepStarted, stepCompleted *time.Time
				if err := rows.Scan(&stepName, &stepStatus, &stepError, &stepStarted, &stepCompleted); err != nil {
					return fmt.Errorf("scanning step: %w", err)
				}

				duration := ""
				if stepStarted != nil && stepCompleted != nil {
					duration = fmt.Sprintf(" (%s)", stepCompleted.Sub(*stepStarted).Round(time.Millisecond))
				}

				fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %s%s\n", stepName, stepStatus, duration)
				if stepError != nil && *stepError != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "    error: %s\n", *stepError)
				}
			}

			return rows.Err()
		},
	}
}
