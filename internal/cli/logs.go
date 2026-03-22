package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/auth"
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

	database, err := db.Open(cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer database.Close()

	// Fetch execution info.
	var workflowName string
	var version int
	var status string
	var startedAt, completedAt *time.Time
	teamID := auth.TeamIDFromContext(cmd.Context())
	err = database.QueryRowContext(cmd.Context(),
		`SELECT workflow_name, workflow_version, status, started_at, completed_at
		 FROM workflow_executions WHERE id = $1 AND team_id = $2`, execID, teamID,
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

	// Fetch top-level step executions.
	rows, err := database.QueryContext(cmd.Context(),
		`SELECT id, step_name, status, error, started_at, completed_at
		 FROM step_executions WHERE execution_id = $1 AND parent_step_id IS NULL
		 ORDER BY created_at ASC`, execID,
	)
	if err != nil {
		return fmt.Errorf("querying steps: %w", err)
	}
	defer rows.Close()

	// Fetch sub-steps grouped by parent.
	type subStep struct {
		StepName  string
		Status    string
		Started   *time.Time
		Completed *time.Time
	}
	subStepsByParent := make(map[string][]subStep)
	subRows, err := database.QueryContext(cmd.Context(),
		`SELECT parent_step_id, step_name, status, started_at, completed_at
		 FROM step_executions WHERE execution_id = $1 AND parent_step_id IS NOT NULL
		 ORDER BY created_at ASC`, execID,
	)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var parentID, sName, sStatus string
			var sStarted, sCompleted *time.Time
			if err := subRows.Scan(&parentID, &sName, &sStatus, &sStarted, &sCompleted); err == nil {
				subStepsByParent[parentID] = append(subStepsByParent[parentID], subStep{sName, sStatus, sStarted, sCompleted})
			}
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Steps:")
	for rows.Next() {
		var stepID, stepName, stepStatus string
		var stepError *string
		var stepStarted, stepCompleted *time.Time
		if err := rows.Scan(&stepID, &stepName, &stepStatus, &stepError, &stepStarted, &stepCompleted); err != nil {
			return fmt.Errorf("scanning step: %w", err)
		}

		icon := statusIcon(stepStatus)
		duration := ""
		if stepStarted != nil && stepCompleted != nil {
			duration = fmt.Sprintf(" %s", stepCompleted.Sub(*stepStarted).Round(time.Millisecond))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "  %s %-20s %s%s\n", icon, stepName, stepStatus, duration)
		if stepError != nil && *stepError != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "      error: %s\n", *stepError)
		}

		// Render sub-steps (tool calls) as indented tree.
		if subs, ok := subStepsByParent[stepID]; ok && len(subs) > 0 {
			// Group by round (last segment of step name: parent/tool/name/round).
			rounds := make(map[string][]subStep)
			var roundOrder []string
			for _, s := range subs {
				parts := strings.Split(s.StepName, "/")
				round := "0"
				if len(parts) >= 4 {
					round = parts[len(parts)-1]
				}
				if _, seen := rounds[round]; !seen {
					roundOrder = append(roundOrder, round)
				}
				rounds[round] = append(rounds[round], s)
			}
			for ri, round := range roundOrder {
				isLastRound := ri == len(roundOrder)-1
				connector := "├─"
				if isLastRound {
					connector = "└─"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "      %s round %s\n", connector, round)

				for si, s := range rounds[round] {
					isLast := si == len(rounds[round])-1
					prefix := "│  "
					if isLastRound {
						prefix = "   "
					}
					subConn := "├─"
					if isLast {
						subConn = "└─"
					}
					toolName := s.StepName
					parts := strings.Split(s.StepName, "/")
					if len(parts) >= 3 {
						toolName = parts[2]
					}
					subDur := ""
					if s.Started != nil && s.Completed != nil {
						subDur = fmt.Sprintf(" %s", s.Completed.Sub(*s.Started).Round(time.Millisecond))
					}
					fmt.Fprintf(cmd.OutOrStdout(), "      %s %s %-16s %s%s\n", prefix, subConn, toolName, s.Status, subDur)
				}
			}
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

	database, err := db.Open(cfg.Database)
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

func statusIcon(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "running":
		return "▶"
	case "skipped":
		return "○"
	case "cancelled":
		return "■"
	default:
		return "○"
	}
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
