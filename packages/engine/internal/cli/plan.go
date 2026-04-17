package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

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
		Long: `Diffs a local workflow definition against the currently applied version
and shows what will change. When --values or --env is supplied, the resolved
input values and env variables that the next run would use are appended to
the output along with their source layer — useful for verifying promotion
targets in CI before apply.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			var valuesInputs map[string]any
			var valuesEnv map[string]string
			if valuesFile != "" {
				vals, valErr := workflow.LoadValues(valuesFile)
				if valErr != nil {
					return fmt.Errorf("loading values file: %w", valErr)
				}
				valuesInputs = vals.Inputs
				valuesEnv = vals.Env
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			var envInputs map[string]any
			var envEnvVars map[string]string
			if envName != "" {
				envStore := &environment.Store{DB: database, Actor: "cli"}
				storedEnv, envErr := envStore.Get(cmd.Context(), envName)
				if envErr != nil {
					return fmt.Errorf("resolving environment %q: %w", envName, envErr)
				}
				envInputs = storedEnv.Inputs
				envEnvVars = storedEnv.Env
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

			storedContent, storedVersion, err := workflow.GetLatestContent(cmd.Context(), database, name)
			if err != nil {
				return fmt.Errorf("fetching latest version: %w", err)
			}

			out := cmd.OutOrStdout()

			if storedContent == nil {
				diff := workflow.Diff(nil, result.Workflow)
				fmt.Fprint(out, diff)
				fmt.Fprintf(out, "\nPlan: 1 workflow to create\n")
				writeResolvedOverrides(out, result.Workflow.Inputs, envInputs, valuesInputs, cfg.Env, envEnvVars, valuesEnv)
				return nil
			}

			h := sha256.Sum256(rawContent)
			newHash := hex.EncodeToString(h[:])
			storedHash, err := workflow.GetLatestHash(cmd.Context(), database, name)
			if err != nil {
				return fmt.Errorf("fetching hash: %w", err)
			}
			if newHash == storedHash {
				fmt.Fprintf(out, "No changes — %s is at version %d\n", name, storedVersion)
				writeResolvedOverrides(out, result.Workflow.Inputs, envInputs, valuesInputs, cfg.Env, envEnvVars, valuesEnv)
				return nil
			}

			var oldWorkflow workflow.Workflow
			if err := json.Unmarshal(storedContent, &oldWorkflow); err != nil {
				return fmt.Errorf("unmarshaling stored workflow: %w", err)
			}

			diff := workflow.Diff(&oldWorkflow, result.Workflow)
			if diff == "" {
				fmt.Fprintf(out, "No changes — %s is at version %d\n", name, storedVersion)
				writeResolvedOverrides(out, result.Workflow.Inputs, envInputs, valuesInputs, cfg.Env, envEnvVars, valuesEnv)
				return nil
			}

			fmt.Fprint(out, diff)
			fmt.Fprintf(out, "\nPlan: 1 workflow to update (version %d → %d)\n", storedVersion, storedVersion+1)
			writeResolvedOverrides(out, result.Workflow.Inputs, envInputs, valuesInputs, cfg.Env, envEnvVars, valuesEnv)
			return nil
		},
	}

	cmd.Flags().StringVar(&valuesFile, "values", "", "Values file with input and env overrides (YAML). When set, plan appends the resolved configuration.")
	cmd.Flags().StringVar(&envName, "env", "", "Named environment to apply. When set, plan appends the resolved configuration and its source per value.")
	return cmd
}

// writeResolvedOverrides appends a "Resolved configuration" block showing the
// final inputs and env vars the next run would see. Only writes if at least
// one override layer was provided.
func writeResolvedOverrides(
	w io.Writer,
	workflowInputs map[string]workflow.Input,
	envInputs map[string]any,
	valuesInputs map[string]any,
	configEnv, envEnvVars, valuesEnv map[string]string,
) {
	if envInputs == nil && valuesInputs == nil && envEnvVars == nil && valuesEnv == nil {
		return
	}

	fmt.Fprintln(w, "\nResolved configuration:")

	inputs := workflow.ResolveInputs(workflowInputs, envInputs, valuesInputs, nil)
	if len(inputs) > 0 {
		fmt.Fprintln(w, "  Inputs:")
		keys := make([]string, 0, len(inputs))
		for k := range inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "    %s = %v  (from: %s)\n", k, inputs[k].Value, inputs[k].Source)
		}
	}

	envs := workflow.ResolveEnvVars(configEnv, envEnvVars, valuesEnv)
	if len(envs) > 0 {
		fmt.Fprintln(w, "  Env vars:")
		keys := make([]string, 0, len(envs))
		for k := range envs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "    %s = %q  (from: %s)\n", k, envs[k].Value, envs[k].Source)
		}
	}

	fmt.Fprintln(w, "  (Inline --input flags, if any, would override these at run time. MANTLE_ENV_* OS vars override at run time too.)")
}
