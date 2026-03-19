package cli

import (
	"github.com/dvflw/mantle/internal/config"
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root mantle CLI command.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mantle",
		Short:        "Headless AI workflow automation platform",
		Long:         "Mantle is a headless AI workflow automation platform — BYOK, IaC-first, enterprise-grade, open source.",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cmd)
			if err != nil {
				return err
			}
			cmd.SetContext(config.WithContext(cmd.Context(), cfg))
			return nil
		},
	}

	cmd.PersistentFlags().String("config", "", "config file path (default: mantle.yaml)")
	cmd.PersistentFlags().String("database-url", "", "database connection URL")
	cmd.PersistentFlags().String("api-address", "", "API listen address")
	cmd.PersistentFlags().String("log-level", "", "log level (debug, info, warn, error)")

	cmd.AddCommand(newVersionCommand())
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newMigrateCommand())
	cmd.AddCommand(newValidateCommand())
	cmd.AddCommand(newPlanCommand())
	cmd.AddCommand(newApplyCommand())
	cmd.AddCommand(newRunCommand())
	cmd.AddCommand(newCancelCommand())
	cmd.AddCommand(newLogsCommand())
	cmd.AddCommand(newStatusCommand())

	return cmd
}
