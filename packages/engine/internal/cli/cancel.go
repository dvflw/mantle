package cli

import (
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/audit"
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

			tx, err := database.BeginTx(cmd.Context(), nil)
			if err != nil {
				return fmt.Errorf("starting transaction: %w", err)
			}
			defer tx.Rollback() //nolint:errcheck

			// Cancel the execution and all child executions recursively.
			// Use RETURNING id to capture which executions were cancelled.
			rows, err := tx.QueryContext(cmd.Context(),
				`WITH RECURSIVE children AS (
					SELECT id FROM workflow_executions WHERE id = $1
					UNION ALL
					SELECT e.id FROM workflow_executions e
					JOIN children c ON e.parent_execution_id = c.id
				)
				UPDATE workflow_executions SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
				WHERE id IN (SELECT id FROM children) AND status IN ('pending', 'running', 'queued')
				RETURNING id`,
				execID,
			)
			if err != nil {
				return fmt.Errorf("cancelling execution: %w", err)
			}

			var cancelledIDs []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return fmt.Errorf("scanning cancelled id: %w", err)
				}
				cancelledIDs = append(cancelledIDs, id)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating cancelled ids: %w", err)
			}

			if len(cancelledIDs) == 0 {
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
			_, err = tx.ExecContext(cmd.Context(),
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
			if err != nil {
				return fmt.Errorf("cancelling step executions: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("committing cancellation: %w", err)
			}

			// Emit audit events for each cancelled execution.
			auditor := &audit.PostgresEmitter{DB: database}
			for _, id := range cancelledIDs {
				_ = auditor.Emit(cmd.Context(), audit.Event{
					Timestamp: time.Now(),
					Actor:     "cli",
					Action:    audit.ActionExecutionCancelled,
					Resource:  audit.Resource{Type: "workflow_execution", ID: id},
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Cancelled execution %s (%d executions affected)\n", execID, len(cancelledIDs))
			return nil
		},
	}
}
