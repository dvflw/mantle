package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/spf13/cobra"
)

func newSecretsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage encrypted credentials",
		Long:  "Create, list, and delete encrypted credentials used by workflow connectors.",
	}

	cmd.AddCommand(newSecretsCreateCommand())
	cmd.AddCommand(newSecretsListCommand())
	cmd.AddCommand(newSecretsDeleteCommand())
	cmd.AddCommand(newSecretsRotateKeyCommand())

	return cmd
}

func newSecretsCreateCommand() *cobra.Command {
	var name string
	var typeName string
	var fields []string

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a new credential",
		Long:    "Creates a new encrypted credential with the specified type and field values.",
		Example: "  mantle secrets create --name openai --type openai --field api_key=sk-...",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newSecretStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			data := make(map[string]string)
			for _, f := range fields {
				k, v, ok := strings.Cut(f, "=")
				if !ok {
					return fmt.Errorf("invalid field format %q: expected key=value", f)
				}
				data[k] = v
			}

			cred, err := store.Create(cmd.Context(), name, typeName, data)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created credential %q (type: %s)\n", cred.Name, cred.Type)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "credential name (required)")
	cmd.Flags().StringVar(&typeName, "type", "", "credential type: "+strings.Join(secret.ListTypes(), ", ")+" (required)")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "field value as key=value (repeatable)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func newSecretsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all credentials",
		Long:  "Lists all stored credentials showing name, type, and creation date. Never shows decrypted values.",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newSecretStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			creds, err := store.List(cmd.Context())
			if err != nil {
				return err
			}

			if len(creds) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no credentials)")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tCREATED")
			for _, c := range creds {
				fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, c.Type, c.CreatedAt.Format("2006-01-02 15:04:05"))
			}
			return w.Flush()
		},
	}
}

func newSecretsDeleteCommand() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a credential",
		Long:  "Permanently deletes an encrypted credential by name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newSecretStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			if err := store.Delete(cmd.Context(), name); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted credential %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "credential name to delete (required)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// newSecretStore builds a secret.Store from the current command context.
// It returns the store, a cleanup function to close the database, and any error.
func newSecretStore(cmd *cobra.Command) (*secret.Store, func(), error) {
	cfg := config.FromContext(cmd.Context())
	if cfg == nil {
		return nil, nil, fmt.Errorf("config not loaded")
	}

	if cfg.Encryption.Key == "" {
		return nil, nil, fmt.Errorf("encryption key not configured (set MANTLE_ENCRYPTION_KEY or encryption.key in mantle.yaml)")
	}

	enc, err := secret.NewEncryptor(cfg.Encryption.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing encryptor: %w", err)
	}

	database, err := db.Open(cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &secret.Store{DB: database, Encryptor: enc}
	cleanup := func() { database.Close() }
	return store, cleanup, nil
}
