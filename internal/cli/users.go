package cli

import (
	"database/sql"
	"fmt"
	"text/tabwriter"

	"github.com/dvflw/mantle/internal/auth"
	"github.com/spf13/cobra"
)

func newUsersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage users",
		Long:  "Create, list, and delete users, and manage API keys.",
	}

	cmd.AddCommand(newUsersCreateCommand())
	cmd.AddCommand(newUsersListCommand())
	cmd.AddCommand(newUsersDeleteCommand())
	cmd.AddCommand(newUsersAPIKeyCommand())

	return cmd
}

func newUsersCreateCommand() *cobra.Command {
	var email, name, team, role string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new user",
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

			user, err := store.CreateUser(cmd.Context(), email, name, t.ID, r)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created user %s (role: %s, team: %s)\n", user.Email, user.Role, team)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "user email (required)")
	cmd.Flags().StringVar(&name, "name", "", "user display name (required)")
	cmd.Flags().StringVar(&team, "team", "default", "team name")
	cmd.Flags().StringVar(&role, "role", "operator", "role: admin, team_owner, operator")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newUsersListCommand() *cobra.Command {
	var team string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users in a team",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newAuthStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			t, err := store.GetTeamByName(cmd.Context(), team)
			if err != nil {
				return err
			}

			users, err := store.ListUsers(cmd.Context(), t.ID)
			if err != nil {
				return err
			}

			if len(users) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no users)")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "EMAIL\tNAME\tROLE")
			for _, u := range users {
				fmt.Fprintf(w, "%s\t%s\t%s\n", u.Email, u.Name, u.Role)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&team, "team", "default", "team name to list users for")

	return cmd
}

func newUsersDeleteCommand() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a user",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newAuthStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			if err := store.DeleteUser(cmd.Context(), email); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted user %q\n", email)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "user email to delete (required)")
	_ = cmd.MarkFlagRequired("email")

	return cmd
}

func newUsersAPIKeyCommand() *cobra.Command {
	var email, keyName string

	cmd := &cobra.Command{
		Use:   "api-key",
		Short: "Create an API key for a user",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newAuthStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			// Look up user ID by email.
			var userID string
			err = store.DB.QueryRowContext(cmd.Context(),
				`SELECT id FROM users WHERE email = $1`, email,
			).Scan(&userID)
			if err == sql.ErrNoRows {
				return fmt.Errorf("user %q not found", email)
			}
			if err != nil {
				return fmt.Errorf("looking up user: %w", err)
			}

			rawKey, apiKey, err := store.CreateAPIKey(cmd.Context(), userID, keyName)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "")
			fmt.Fprintf(out, "API Key: %s\n", rawKey)
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "Save this key — it cannot be retrieved again.")
			fmt.Fprintf(out, "Key prefix for reference: %s\n", apiKey.KeyPrefix)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "user email (required)")
	cmd.Flags().StringVar(&keyName, "key-name", "", "API key name (required)")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("key-name")

	return cmd
}

// parseRole validates and converts a string to an auth.Role.
func parseRole(s string) (auth.Role, error) {
	switch auth.Role(s) {
	case auth.RoleAdmin, auth.RoleTeamOwner, auth.RoleOperator:
		return auth.Role(s), nil
	default:
		return "", fmt.Errorf("invalid role %q: must be admin, team_owner, or operator", s)
	}
}
