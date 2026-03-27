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
		Long: `Mantle is a headless AI workflow automation platform.
BYOK, IaC-first, enterprise-grade, source-available.

  validate → plan → apply → run

Full documentation: https://mantle.dvflw.co/docs`,
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
	cmd.PersistentFlags().String("api-key", "", "API key for authentication (overrides cached credentials)")
	cmd.PersistentFlags().StringP("output", "o", "text", "Output format: text or json")

	// Command groups for organized help output.
	cmd.AddGroup(
		&cobra.Group{ID: "workflow", Title: "Workflow Lifecycle:"},
		&cobra.Group{ID: "server", Title: "Server & Triggers:"},
		&cobra.Group{ID: "auth", Title: "Authentication & Secrets:"},
		&cobra.Group{ID: "admin", Title: "Administration:"},
		&cobra.Group{ID: "info", Title: "Info:"},
	)

	// Workflow lifecycle commands.
	addToGroup(cmd, "workflow",
		newValidateCommand(),
		newPlanCommand(),
		newApplyCommand(),
		newRunCommand(),
		newRetryCommand(),
		newRollbackCommand(),
		newCancelCommand(),
		newLogsCommand(),
		newStatusCommand(),
	)

	// Server & triggers.
	addToGroup(cmd, "server",
		newServeCommand(),
	)

	// Authentication & secrets.
	addToGroup(cmd, "auth",
		newSecretsCommand(),
		newLoginCommand(),
		newLogoutCommand(),
	)

	// Administration.
	addToGroup(cmd, "admin",
		newInitCommand(),
		newMigrateCommand(),
		newTeamsCommand(),
		newUsersCommand(),
		newRolesCommand(),
		newAuditCommand(),
		newPluginsCommand(),
		newLibraryCommand(),
		newCleanupCommand(),
		newBudgetCommand(),
	)

	// Info.
	addToGroup(cmd, "info",
		newVersionCommand(),
	)

	return cmd
}

// addToGroup sets GroupID on each command and adds it to the parent.
func addToGroup(parent *cobra.Command, groupID string, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = groupID
		parent.AddCommand(c)
	}
}
