package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/environment"
	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

func newPlanCommand() *cobra.Command {
	var valuesFile string
	var envName string

	cmd := &cobra.Command{
		Use:   "plan <file>",
		Short: "Show what will change",
		Long:  "Diffs a local workflow definition against the currently applied version and shows what will change.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			// Load values file early so we fail fast on bad input.
			if valuesFile != "" {
				_, valErr := workflow.LoadValues(valuesFile)
				if valErr != nil {
					return fmt.Errorf("loading values file: %w", valErr)
				}
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			if envName != "" {
				envStore := &environment.Store{DB: database}
				if _, envErr := envStore.Get(cmd.Context(), envName); envErr != nil {
					return fmt.Errorf("resolving environment %q: %w", envName, envErr)
				}
			}

			rawContent, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Errorf("reading %s: %w", filename, err)
			}

			result, err := workflow.ParseBytes(rawContent)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", filename, err)
			}

			errs := workflow.Validate(result)
			if len(errs) > 0 {
				return fmt.Errorf("validation failed: %v", errs[0])
			}

			name := result.Workflow.Name

			// Fetch latest stored version.
			storedContent, storedVersion, err := workflow.GetLatestContent(cmd.Context(), database, name)
			if err != nil {
				return fmt.Errorf("fetching latest version: %w", err)
			}

			// New workflow — no prior version.
			if storedContent == nil {
				diff := workflow.Diff(nil, result.Workflow)
				fmt.Fprint(cmd.OutOrStdout(), diff)
				fmt.Fprintf(cmd.OutOrStdout(), "\nPlan: 1 workflow to create\n")
				return nil
			}

			// Check if content is unchanged.
			h := sha256.Sum256(rawContent)
			newHash := hex.EncodeToString(h[:])
			storedHash, err := workflow.GetLatestHash(cmd.Context(), database, name)
			if err != nil {
				return fmt.Errorf("fetching hash: %w", err)
			}
			if newHash == storedHash {
				fmt.Fprintf(cmd.OutOrStdout(), "No changes — %s is at version %d\n", name, storedVersion)
				return nil
			}

			// Diff against stored version.
			var oldWorkflow workflow.Workflow
			if err := json.Unmarshal(storedContent, &oldWorkflow); err != nil {
				return fmt.Errorf("unmarshaling stored workflow: %w", err)
			}

			diff := workflow.Diff(&oldWorkflow, result.Workflow)
			if diff == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "No changes — %s is at version %d\n", name, storedVersion)
				return nil
			}

			fmt.Fprint(cmd.OutOrStdout(), diff)
			fmt.Fprintf(cmd.OutOrStdout(), "\nPlan: 1 workflow to update (version %d → %d)\n", storedVersion, storedVersion+1)
			return nil
		},
	}

	cmd.Flags().StringVar(&valuesFile, "values", "", "Values file with input and env overrides (YAML)")
	cmd.Flags().StringVar(&envName, "env", "", "Named environment (validates it exists)")
	return cmd
}
