package cli

import (
	"encoding/json"
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/engine"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/spf13/cobra"
)

func newRetryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retry <execution-id>",
		Short: "Retry a failed workflow execution",
		Long: `Creates a new execution that resumes from the failure point of a
previous execution. Completed upstream steps are copied; the failed step
and everything downstream re-execute.`,
		Example: `  mantle retry 01234567-89ab-cdef-0123-456789abcdef
  mantle retry 01234567-89ab-cdef-0123-456789abcdef --from-step process-data
  mantle retry 01234567-89ab-cdef-0123-456789abcdef --force`,
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

			fromStep, _ := cmd.Flags().GetString("from-step")
			force, _ := cmd.Flags().GetBool("force")
			outputFormat, _ := cmd.Flags().GetString("output")
			verbose, _ := cmd.Flags().GetBool("verbose")

			if outputFormat != "json" {
				fmt.Fprintf(cmd.OutOrStdout(), "Retrying execution %s", execID)
				if fromStep != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " from step %q", fromStep)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "...")
			}

			result, err := eng.RetryExecution(cmd.Context(), execID, fromStep, force)
			if err != nil {
				return fmt.Errorf("retry failed: %w", err)
			}

			// Compute the exit error before writing output so that JSON
			// mode exits non-zero on failure/timeout/cancellation.
			var exitErr error
			switch result.Status {
			case "failed", "timed_out", "cancelled":
				failedStep := ""
				for _, s := range orderedSteps(result) {
					if s.status == "failed" {
						failedStep = s.name
						break
					}
				}
				if failedStep != "" {
					exitErr = fmt.Errorf("workflow %s at step %q: %s", result.Status, failedStep, result.Error)
				} else {
					exitErr = fmt.Errorf("workflow %s: %s", result.Status, result.Error)
				}
			}

			// JSON output mode.
			if outputFormat == "json" {
				if encErr := json.NewEncoder(cmd.OutOrStdout()).Encode(result); encErr != nil {
					return fmt.Errorf("encoding JSON output: %w", encErr)
				}
				return exitErr
			}

			// Text output mode.
			fmt.Fprintf(cmd.OutOrStdout(), "Execution %s: %s (%s)\n",
				result.ExecutionID, result.Status, formatDuration(result.Duration))

			steps := orderedSteps(result)
			if verbose {
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

			return exitErr
		},
	}

	cmd.Flags().String("from-step", "", "Step to retry from (default: first failed step)")
	cmd.Flags().Bool("force", false, "Bypass concurrency limits")
	cmd.Flags().BoolP("verbose", "v", false, "Show step outputs and durations")
	return cmd
}
