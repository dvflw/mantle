package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [execution-id]",
		Short: "View execution logs",
		Long: `Shows step-by-step execution history with timing, status, and outputs.

When called with an execution ID, shows detailed step information.
When called without arguments, lists recent executions with optional filters.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return showExecutionDetail(cmd, args[0])
			}
			return listExecutions(cmd)
		},
	}

	cmd.Flags().String("workflow", "", "filter by workflow name")
	cmd.Flags().String("status", "", "filter by status (pending, running, completed, failed, cancelled)")
	cmd.Flags().String("since", "", "filter by time (e.g., 1h, 24h, 7d)")
	cmd.Flags().Int("limit", 20, "max results to return")

	return cmd
}

// showExecutionDetail displays detailed step info for a single execution (original behavior).
func showExecutionDetail(cmd *cobra.Command, execID string) error {
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
}

// listExecutions lists recent executions with optional filters.
func listExecutions(cmd *cobra.Command) error {
	cfg := config.FromContext(cmd.Context())
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	database, err := db.Open(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer database.Close()

	workflow, _ := cmd.Flags().GetString("workflow")
	status, _ := cmd.Flags().GetString("status")
	since, _ := cmd.Flags().GetString("since")
	limit, _ := cmd.Flags().GetInt("limit")

	if limit <= 0 {
		limit = 20
	}

	// Validate status if provided.
	if status != "" {
		validStatuses := map[string]bool{
			"pending": true, "running": true, "completed": true,
			"failed": true, "cancelled": true,
		}
		if !validStatuses[strings.ToLower(status)] {
			return fmt.Errorf("invalid status %q: must be one of pending, running, completed, failed, cancelled", status)
		}
		status = strings.ToLower(status)
	}

	// Build query dynamically with parameterized filters.
	query := `SELECT id, workflow_name, workflow_version, status, started_at, completed_at
		 FROM workflow_executions WHERE 1=1`
	params := []any{}
	paramIdx := 1

	if workflow != "" {
		query += " AND workflow_name = $" + strconv.Itoa(paramIdx)
		params = append(params, workflow)
		paramIdx++
	}

	if status != "" {
		query += " AND status = $" + strconv.Itoa(paramIdx)
		params = append(params, status)
		paramIdx++
	}

	if since != "" {
		duration, err := parseDuration(since)
		if err != nil {
			return fmt.Errorf("invalid --since value %q: %w", since, err)
		}
		cutoff := time.Now().Add(-duration)
		query += " AND started_at >= $" + strconv.Itoa(paramIdx)
		params = append(params, cutoff)
		paramIdx++
	}

	query += " ORDER BY started_at DESC NULLS LAST"
	query += " LIMIT $" + strconv.Itoa(paramIdx)
	params = append(params, limit)

	rows, err := database.QueryContext(cmd.Context(), query, params...)
	if err != nil {
		return fmt.Errorf("querying executions: %w", err)
	}
	defer rows.Close()

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-38s %-20s %7s %-10s %-20s %-20s\n",
		"ID", "WORKFLOW", "VERSION", "STATUS", "STARTED", "COMPLETED")
	fmt.Fprintln(out, strings.Repeat("-", 120))

	count := 0
	for rows.Next() {
		var id, wfName, wfStatus string
		var version int
		var startedAt, completedAt *time.Time
		if err := rows.Scan(&id, &wfName, &version, &wfStatus, &startedAt, &completedAt); err != nil {
			return fmt.Errorf("scanning execution: %w", err)
		}

		started := "-"
		if startedAt != nil {
			started = startedAt.Format("2006-01-02 15:04:05")
		}
		completed := "-"
		if completedAt != nil {
			completed = completedAt.Format("2006-01-02 15:04:05")
		}

		fmt.Fprintf(out, "%-38s %-20s %7d %-10s %-20s %-20s\n",
			id, wfName, version, wfStatus, started, completed)
		count++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating executions: %w", err)
	}

	if count == 0 {
		fmt.Fprintln(out, "No executions found.")
	} else {
		fmt.Fprintf(out, "\n%d execution(s) shown.\n", count)
	}

	return nil
}

// parseDuration parses duration strings like "1h", "24h", "7d".
// Supports Go-standard durations plus "d" suffix for days.
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day count: %s", numStr)
		}
		if days <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return d, nil
}
