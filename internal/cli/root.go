package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root mantle CLI command.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mantle",
		Short:        "Headless AI workflow automation platform",
		Long:         "Mantle is a headless AI workflow automation platform — BYOK, IaC-first, enterprise-grade, open source.",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().String("config", "", "config file path (default: mantle.yaml)")

	cmd.AddCommand(newVersionCommand())

	return cmd
}
