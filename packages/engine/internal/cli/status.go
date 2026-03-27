package cli

import (
	"encoding/json"
	"fmt"
	"strings"
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
		Example: `  mantle status abc123
  mantle status abc123 --output json`,
		Args: cobra.ExactArgs(1),
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

			// Step summary.
			rows, err := database.QueryContext(cmd.Context(),
				`SELECT status, COUNT(*) FROM step_executions
				 WHERE execution_id = $1 GROUP BY status`, execID,
			)
			if err != nil {
				return fmt.Errorf("querying step counts: %w", err)
			}
			defer rows.Close()

			stepCounts := make(map[string]int)
			for rows.Next() {
				var stepStatus string
				var count int
				if err := rows.Scan(&stepStatus, &count); err != nil {
					return fmt.Errorf("scanning: %w", err)
				}
				stepCounts[stepStatus] = count
			}
			if err := rows.Err(); err != nil {
				return err
			}

			// JSON output mode.
			outputFormat, _ := cmd.Flags().GetString("output")
			if outputFormat == "json" {
				detail := map[string]any{
					"execution_id": execID,
					"workflow":     workflowName,
					"version":      version,
					"status":       status,
					"started_at":   startedAt,
					"completed_at": completedAt,
					"step_counts":  stepCounts,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(detail)
			}

			// Text output.
			fmt.Fprintf(cmd.OutOrStdout(), "Execution: %s\n", execID)
			fmt.Fprintf(cmd.OutOrStdout(), "Workflow:  %s (version %d)\n", workflowName, version)
			fmt.Fprintf(cmd.OutOrStdout(), "Status:    %s\n", status)
			if startedAt != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Started:   %s\n", startedAt.Format(time.RFC3339))
			}
			if completedAt != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Completed: %s\n", completedAt.Format(time.RFC3339))
			}

			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Steps:")
			for stepStatus, count := range stepCounts {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %d\n", stepStatus, count)
			}

			// Query for full descendant execution tree using recursive CTE.
			childRows, err := database.QueryContext(cmd.Context(),
				`WITH RECURSIVE tree AS (
					SELECT id, workflow_name, workflow_version, status, 1 as depth
					FROM workflow_executions WHERE parent_execution_id = $1
					UNION ALL
					SELECT e.id, e.workflow_name, e.workflow_version, e.status, t.depth + 1
					FROM workflow_executions e
					JOIN tree t ON e.parent_execution_id = t.id
				)
				SELECT id, workflow_name, workflow_version, status, depth FROM tree ORDER BY depth, id`, execID,
			)
			if err == nil {
				defer childRows.Close()

				type childExec struct {
					ID       string
					Workflow string
					Version  int
					Status   string
					Depth    int
				}
				var children []childExec
				for childRows.Next() {
					var c childExec
					if err := childRows.Scan(&c.ID, &c.Workflow, &c.Version, &c.Status, &c.Depth); err == nil {
						children = append(children, c)
					}
				}

				if len(children) > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
					fmt.Fprintln(cmd.OutOrStdout(), "Execution Tree:")
					for _, c := range children {
						icon := statusIcon(c.Status)
						indent := strings.Repeat("  ", c.Depth)
						fmt.Fprintf(cmd.OutOrStdout(), "%s%s %-20s v%-4d %s  %s\n",
							indent, icon, c.Workflow, c.Version, c.Status, c.ID)
					}
				}
			}

			return nil
		},
	}
}
