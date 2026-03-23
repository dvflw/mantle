package cli

import (
	"encoding/json"
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
		Example: `  mantle logs abc123
  mantle logs --workflow my-workflow --status failed
  mantle logs --since 24h --limit 10
  mantle logs abc123 --output json`,
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
		StepName  string     `json:"step_name"`
		Status    string     `json:"status"`
		Started   *time.Time `json:"started_at,omitempty"`
		Completed *time.Time `json:"completed_at,omitempty"`
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

	// Collect step data.
	type stepInfo struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Status    string    `json:"status"`
		Error     string    `json:"error,omitempty"`
		StartedAt *time.Time `json:"started_at,omitempty"`
		CompletedAt *time.Time `json:"completed_at,omitempty"`
		SubSteps  []subStep  `json:"sub_steps,omitempty"`
	}

	var stepsData []stepInfo
	for rows.Next() {
		var stepID, stepName, stepStatus string
		var stepError *string
		var stepStarted, stepCompleted *time.Time
		if err := rows.Scan(&stepID, &stepName, &stepStatus, &stepError, &stepStarted, &stepCompleted); err != nil {
			return fmt.Errorf("scanning step: %w", err)
		}

		si := stepInfo{
			ID:          stepID,
			Name:        stepName,
			Status:      stepStatus,
			StartedAt:   stepStarted,
			CompletedAt: stepCompleted,
		}
		if stepError != nil && *stepError != "" {
			si.Error = *stepError
		}
		if subs, ok := subStepsByParent[stepID]; ok {
			si.SubSteps = subs
		}
		stepsData = append(stepsData, si)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating steps: %w", err)
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
			"steps":        stepsData,
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
		if startedAt != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Duration:  %s\n", completedAt.Sub(*startedAt).Round(time.Millisecond))
		}
	}
	fmt.Fprintln(cmd.OutOrStdout())

	fmt.Fprintln(cmd.OutOrStdout(), "Steps:")
	for _, si := range stepsData {
		icon := statusIcon(si.Status)
		duration := ""
		if si.StartedAt != nil && si.CompletedAt != nil {
			duration = fmt.Sprintf(" %s", si.CompletedAt.Sub(*si.StartedAt).Round(time.Millisecond))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "  %s %-20s %s%s\n", icon, si.Name, si.Status, duration)
		if si.Error != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "      error: %s\n", si.Error)
		}

		// Render sub-steps (tool calls) as indented tree.
		if len(si.SubSteps) > 0 {
			// Group by round (last segment of step name: parent/tool/name/round).
			rounds := make(map[string][]subStep)
			var roundOrder []string
			for _, s := range si.SubSteps {
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

	return nil
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
	teamID := auth.TeamIDFromContext(cmd.Context())
	query := `SELECT id, workflow_name, workflow_version, status, started_at, completed_at
		 FROM workflow_executions WHERE team_id = $1`
	params := []any{teamID}
	paramIdx := 2

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

	type execRow struct {
		ID          string     `json:"id"`
		Workflow    string     `json:"workflow"`
		Version     int        `json:"version"`
		Status      string     `json:"status"`
		StartedAt   *time.Time `json:"started_at,omitempty"`
		CompletedAt *time.Time `json:"completed_at,omitempty"`
	}

	var executions []execRow
	for rows.Next() {
		var r execRow
		if err := rows.Scan(&r.ID, &r.Workflow, &r.Version, &r.Status, &r.StartedAt, &r.CompletedAt); err != nil {
			return fmt.Errorf("scanning execution: %w", err)
		}
		executions = append(executions, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating executions: %w", err)
	}

	// JSON output mode.
	outputFormat, _ := cmd.Flags().GetString("output")
	if outputFormat == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(executions)
	}

	// Text output mode.
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-38s %-20s %7s %-10s %-20s %-20s\n",
		"ID", "WORKFLOW", "VERSION", "STATUS", "STARTED", "COMPLETED")
	fmt.Fprintln(out, strings.Repeat("-", 120))

	for _, r := range executions {
		started := "-"
		if r.StartedAt != nil {
			started = r.StartedAt.Format("2006-01-02 15:04:05")
		}
		completed := "-"
		if r.CompletedAt != nil {
			completed = r.CompletedAt.Format("2006-01-02 15:04:05")
		}

		fmt.Fprintf(out, "%-38s %-20s %7d %-10s %-20s %-20s\n",
			r.ID, r.Workflow, r.Version, r.Status, started, completed)
	}

	if len(executions) == 0 {
		fmt.Fprintln(out, "No executions found.")
	} else {
		fmt.Fprintf(out, "\n%d execution(s) shown.\n", len(executions))
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
