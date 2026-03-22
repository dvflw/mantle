package cli

import (
	"fmt"
	"path/filepath"

	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a workflow YAML file",
		Long:  "Checks a workflow definition for schema conformance offline. No database or network connection required.",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil // Skip config loading — validate is fully offline
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]

			result, err := workflow.Parse(filename)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", filepath.Base(filename), err)
				return fmt.Errorf("validation failed")
			}

			errs := workflow.Validate(result)
			if len(errs) > 0 {
				for _, e := range errs {
					if e.Line > 0 {
						fmt.Fprintf(cmd.ErrOrStderr(), "%s:%d:%d: error: %s (%s)\n",
							filepath.Base(filename), e.Line, e.Column, e.Message, e.Field)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "%s: error: %s (%s)\n",
							filepath.Base(filename), e.Message, e.Field)
					}
				}
				return fmt.Errorf("validation failed")
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s: valid\n", filepath.Base(filename))
			return nil
		},
	}
}
