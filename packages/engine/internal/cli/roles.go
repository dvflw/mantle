package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRolesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roles",
		Short: "Manage user roles",
		Long:  "Assign roles to users.",
	}

	cmd.AddCommand(newRolesAssignCommand())

	return cmd
}

func newRolesAssignCommand() *cobra.Command {
	var email, role, team string

	cmd := &cobra.Command{
		Use:   "assign",
		Short: "Assign a role to a user",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newAuthStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			r, err := parseRole(role)
			if err != nil {
				return err
			}

			t, err := store.GetTeamByName(cmd.Context(), team)
			if err != nil {
				return err
			}

			if err := store.SetUserRole(cmd.Context(), email, t.ID, r); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Assigned role %q to user %q\n", r, email)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "user email (required)")
	cmd.Flags().StringVar(&role, "role", "", "role: admin, team_owner, operator (required)")
	cmd.Flags().StringVar(&team, "team", "default", "team name")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("role")

	return cmd
}
