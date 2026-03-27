package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/engine"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	var inputFlags []string

	cmd := &cobra.Command{
		Use:   "run <workflow>",
		Short: "Run a workflow",
		Long:  "Triggers execution of a workflow, pinned to the current version.",
		Example: `  mantle run my-workflow
  mantle run my-workflow --input url=https://example.com
  mantle run my-workflow --input url=https://example.com --verbose
  mantle run my-workflow --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowName := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			// Get latest version.
			version, err := workflow.GetLatestVersion(cmd.Context(), database, workflowName)
			if err != nil {
				return fmt.Errorf("looking up workflow: %w", err)
			}
			if version == 0 {
				return fmt.Errorf("workflow %q not found — have you run 'mantle apply'?", workflowName)
			}

			// Parse --input flags into a map.
			inputs := make(map[string]any)
			for _, kv := range inputFlags {
				key, value, ok := strings.Cut(kv, "=")
				if !ok {
					return fmt.Errorf("invalid input format %q — expected key=value", kv)
				}
				inputs[key] = value
			}

			eng, err := engine.New(database)
			if err != nil {
				return fmt.Errorf("creating engine: %w", err)
			}
			eng.MaxConcurrentExecutionsPerTeam = cfg.Engine.MaxConcurrentExecutionsPerTeam

			// Configure credential resolver with Postgres-backed store when encryption key is set.
			if cfg.Encryption.Key != "" {
				encryptor, encErr := secret.NewEncryptor(cfg.Encryption.Key)
				if encErr != nil {
					return fmt.Errorf("configuring encryption: %w", encErr)
				}
				eng.Resolver = &secret.Resolver{
					Store: &secret.Store{DB: database, Encryptor: encryptor},
				}
			}

			outputFormat, _ := cmd.Flags().GetString("output")
			verbose, _ := cmd.Flags().GetBool("verbose")
			force, _ := cmd.Flags().GetBool("force")

			if outputFormat != "json" {
				fmt.Fprintf(cmd.OutOrStdout(), "Running %s (version %d)...\n", workflowName, version)
			}

			result, err := eng.ExecuteWithOptions(cmd.Context(), workflowName, version, inputs, engine.ExecuteOptions{Force: force})
			if err != nil {
				return fmt.Errorf("execution failed: %w", err)
			}

			// JSON output mode.
			if outputFormat == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}

			// Text output mode.
			fmt.Fprintf(cmd.OutOrStdout(), "Execution %s: %s (%s)\n",
				result.ExecutionID, result.Status, formatDuration(result.Duration))

			steps := orderedSteps(result)
			if verbose {
				// Compute max step name width for alignment.
				maxLen := 0
				for _, s := range steps {
					if len(s.name) > maxLen {
						maxLen = len(s.name)
					}
				}
				for _, step := range steps {
					line := fmt.Sprintf("  %s %-*s  %s (%s)",
						statusIcon(step.status), maxLen, step.name+":", step.status, formatDuration(step.duration))
					if step.output != "" {
						line += fmt.Sprintf(" -> %s", truncate(step.output, 500))
					}
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
			} else {
				for _, step := range steps {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s %s: %s\n", statusIcon(step.status), step.name, step.status)
				}
			}

			if result.Status == "failed" {
				// Find the failed step name for a more actionable error message.
				failedStep := ""
				for name, sr := range result.Steps {
					if sr.Status == "failed" {
						failedStep = name
						break
					}
				}
				if failedStep != "" {
					return fmt.Errorf("workflow failed at step %q: %s", failedStep, result.Error)
				}
				return fmt.Errorf("workflow failed: %s", result.Error)
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVar(&inputFlags, "input", nil, "Input parameter (key=value), can be specified multiple times")
	cmd.Flags().BoolP("verbose", "v", false, "Show step outputs and durations")
	cmd.Flags().Bool("force", false, "Bypass concurrency limits")
	return cmd
}

type stepSummary struct {
	name     string
	status   string
	duration time.Duration
	output   string
}

func orderedSteps(result *engine.ExecutionResult) []stepSummary {
	steps := make([]stepSummary, 0, len(result.Steps))
	for name, sr := range result.Steps {
		outputStr := ""
		if sr.Output != nil {
			if data, err := json.Marshal(sr.Output); err == nil {
				outputStr = string(data)
			}
		}
		steps = append(steps, stepSummary{
			name:     name,
			status:   sr.Status,
			duration: sr.Duration,
			output:   outputStr,
		})
	}
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].name < steps[j].name
	})
	return steps
}

// formatDuration formats a duration for human-readable display (e.g., "3.2s", "150ms").
func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Minute:
		return fmt.Sprintf("%.1fm", d.Minutes())
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
