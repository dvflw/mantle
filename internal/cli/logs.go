package cli

import (
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

type stepRow struct {
	id          string
	stepName    string
	status      string
	startedAt   sql.NullTime
	completedAt sql.NullTime
	parentID    sql.NullString
}

func (s stepRow) duration() string {
	if !s.startedAt.Valid {
		return "-"
	}
	end := time.Now()
	if s.completedAt.Valid {
		end = s.completedAt.Time
	}
	d := end.Sub(s.startedAt.Time)
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func statusIcon(status string) string {
	switch status {
	case "completed":
		return "\u2713"
	case "failed":
		return "\u2717"
	case "running":
		return "\u25b6"
	case "pending":
		return "\u25cb"
	case "cancelled":
		return "\u25a0"
	default:
		return "?"
	}
}

// subStep holds a parsed sub-step with its round number.
type subStep struct {
	row      stepRow
	toolName string
	round    int
}

// parseSubStep extracts tool name and round from a step name like "agent/tool/get_weather/0".
func parseSubStep(s stepRow) subStep {
	parts := strings.Split(s.stepName, "/")
	ss := subStep{row: s}
	if len(parts) >= 4 {
		ss.toolName = parts[2]
		ss.round, _ = strconv.Atoi(parts[len(parts)-1])
	} else if len(parts) >= 2 {
		ss.toolName = parts[len(parts)-1]
	} else {
		ss.toolName = s.stepName
	}
	return ss
}

func newLogsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <execution-id>",
		Short: "View execution step history",
		Long:  "Displays step-by-step execution history including tool-use sub-steps rendered as an indented tree.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			executionID := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			// Fetch top-level steps (parent_step_id IS NULL).
			topRows, err := querySteps(cmd, database, executionID, true)
			if err != nil {
				return err
			}
			if len(topRows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No steps found for execution", executionID)
				return nil
			}

			// Fetch sub-steps (parent_step_id IS NOT NULL).
			subRows, err := querySteps(cmd, database, executionID, false)
			if err != nil {
				return err
			}

			// Group sub-steps by parent step ID.
			subsByParent := make(map[string][]subStep)
			for _, r := range subRows {
				if r.parentID.Valid {
					subsByParent[r.parentID.String] = append(subsByParent[r.parentID.String], parseSubStep(r))
				}
			}

			// Render output.
			w := cmd.OutOrStdout()
			for _, step := range topRows {
				fmt.Fprintf(w, "  %s %-20s %-10s %s\n",
					statusIcon(step.status), step.stepName, step.status, step.duration())

				subs, hasSubs := subsByParent[step.id]
				if !hasSubs {
					continue
				}

				renderSubStepTree(w, subs)
			}

			return nil
		},
	}
}

func querySteps(cmd *cobra.Command, database *sql.DB, executionID string, topLevel bool) ([]stepRow, error) {
	var condition string
	if topLevel {
		condition = "parent_step_id IS NULL"
	} else {
		condition = "parent_step_id IS NOT NULL"
	}

	query := fmt.Sprintf(
		`SELECT id, step_name, status, started_at, completed_at, parent_step_id
		 FROM step_executions
		 WHERE execution_id = $1 AND %s
		 ORDER BY created_at`, condition)

	rows, err := database.QueryContext(cmd.Context(), query, executionID)
	if err != nil {
		return nil, fmt.Errorf("querying step executions: %w", err)
	}
	defer rows.Close()

	var result []stepRow
	for rows.Next() {
		var s stepRow
		if err := rows.Scan(&s.id, &s.stepName, &s.status, &s.startedAt, &s.completedAt, &s.parentID); err != nil {
			return nil, fmt.Errorf("scanning step row: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// renderSubStepTree groups sub-steps by round and renders them as a tree.
func renderSubStepTree(w io.Writer, subs []subStep) {
	// Determine distinct rounds in order.
	roundOrder := []int{}
	roundMap := make(map[int][]subStep)
	seen := make(map[int]bool)
	for _, s := range subs {
		if !seen[s.round] {
			seen[s.round] = true
			roundOrder = append(roundOrder, s.round)
		}
		roundMap[s.round] = append(roundMap[s.round], s)
	}
	sort.Ints(roundOrder)

	maxRound := -1
	if len(roundOrder) > 0 {
		maxRound = roundOrder[len(roundOrder)-1]
	}

	for ri, round := range roundOrder {
		isLastRound := ri == len(roundOrder)-1
		tools := roundMap[round]

		// Determine connector characters.
		var branchChar, contChar string
		if isLastRound {
			branchChar = "\u2514\u2500"
			contChar = "  "
		} else {
			branchChar = "\u251c\u2500"
			contChar = "\u2502 "
		}

		// If this is the last round and there are no tool calls, render as "final response".
		if round == maxRound && allToolNames(tools, "final_response") {
			fmt.Fprintf(w, "    %s final response\n", branchChar)
			continue
		}

		fmt.Fprintf(w, "    %s round %d\n", branchChar, round+1)

		for ti, tool := range tools {
			var toolBranch string
			if ti == len(tools)-1 {
				toolBranch = "\u2514\u2500"
			} else {
				toolBranch = "\u251c\u2500"
			}
			fmt.Fprintf(w, "    %s  %s %-16s %-10s %s\n",
				contChar, toolBranch, tool.toolName, tool.row.status, tool.row.duration())
		}
	}
}

// allToolNames checks if every sub-step in the slice has the given tool name.
func allToolNames(subs []subStep, name string) bool {
	for _, s := range subs {
		if s.toolName != name {
			return false
		}
	}
	return true
}
