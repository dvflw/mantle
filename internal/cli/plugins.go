package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/plugin"
	"github.com/spf13/cobra"
)

func newPluginsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Manage plugins",
		Long:  "List, install, and remove third-party connector plugins.",
	}

	cmd.AddCommand(newPluginsListCommand())
	cmd.AddCommand(newPluginsInstallCommand())
	cmd.AddCommand(newPluginsRemoveCommand())

	return cmd
}

func newPluginsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := newPluginManager()
			if err := mgr.Discover(); err != nil {
				return err
			}

			plugins := mgr.List()
			if len(plugins) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no plugins installed)")
				return nil
			}

			for _, p := range plugins {
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", p.Name, p.Path)
			}
			return nil
		},
	}
}

func newPluginsInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install <path>",
		Short: "Install a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := newPluginManager()
			if err := mgr.Install(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed plugin from %s\n", args[0])
			return nil
		},
	}
}

func newPluginsRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := newPluginManager()
			if err := mgr.Discover(); err != nil {
				return err
			}
			if err := mgr.Remove(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed plugin %s\n", args[0])
			return nil
		},
	}
}

func newPluginManager() *plugin.Manager {
	// Default plugin directory — can be overridden via config in a future version.
	return plugin.NewManager(".mantle/plugins", nil)
}
