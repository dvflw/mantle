package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newCancelCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <execution-id>",
		Short: "Cancel a running workflow",
		Long:  "Cancels a running workflow execution. The execution is marked as cancelled.",
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

			// Cancel the execution and all child executions recursively.
			result, err := database.ExecContext(cmd.Context(),
				`WITH RECURSIVE children AS (
					SELECT id FROM workflow_executions WHERE id = $1
					UNION ALL
					SELECT e.id FROM workflow_executions e
					JOIN children c ON e.parent_execution_id = c.id
				)
				UPDATE workflow_executions SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
				WHERE id IN (SELECT id FROM children) AND status IN ('pending', 'running', 'queued')`,
				execID,
			)
			if err != nil {
				return fmt.Errorf("cancelling execution: %w", err)
			}

			rows, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("checking result: %w", err)
			}

			if rows == 0 {
				// Check if execution exists at all.
				var status string
				err := database.QueryRowContext(cmd.Context(),
					`SELECT status FROM workflow_executions WHERE id = $1`, execID,
				).Scan(&status)
				if err != nil {
					return fmt.Errorf("execution %q not found", execID)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Execution %s is already %s\n", execID, status)
				return nil
			}

			// Also mark any running/pending steps in the tree as cancelled.
			database.ExecContext(cmd.Context(),
				`WITH RECURSIVE children AS (
					SELECT id FROM workflow_executions WHERE id = $1
					UNION ALL
					SELECT e.id FROM workflow_executions e
					JOIN children c ON e.parent_execution_id = c.id
				)
				UPDATE step_executions SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
				WHERE execution_id IN (SELECT id FROM children) AND status IN ('pending', 'running')`,
				execID,
			)

			fmt.Fprintf(cmd.OutOrStdout(), "Cancelled execution %s\n", execID)
			return nil
		},
	}
}
