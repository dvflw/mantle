package cli

import (
	"fmt"
	"sort"
	"strings"

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
		Args:  cobra.ExactArgs(1),
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

			fmt.Fprintf(cmd.OutOrStdout(), "Running %s (version %d)...\n", workflowName, version)

			result, err := eng.Execute(cmd.Context(), workflowName, version, inputs)
			if err != nil {
				return fmt.Errorf("execution failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Execution %s: %s\n", result.ExecutionID, result.Status)

			for _, step := range orderedSteps(result) {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", step.name, step.status)
			}

			if result.Status == "failed" {
				return fmt.Errorf("workflow failed: %s", result.Error)
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVar(&inputFlags, "input", nil, "Input parameter (key=value), can be specified multiple times")
	return cmd
}

type stepSummary struct {
	name   string
	status string
}

func orderedSteps(result *engine.ExecutionResult) []stepSummary {
	steps := make([]stepSummary, 0, len(result.Steps))
	for name, sr := range result.Steps {
		steps = append(steps, stepSummary{name: name, status: sr.Status})
	}
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].name < steps[j].name
	})
	return steps
}
