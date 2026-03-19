package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/secret"
	"github.com/spf13/cobra"
)

func newSecretsRotateKeyCommand() *cobra.Command {
	var newKey string

	cmd := &cobra.Command{
		Use:   "rotate-key",
		Short: "Re-encrypt all credentials with a new master key",
		Long:  "Decrypts all stored credentials with the current master key and re-encrypts them with a new key. Used for key rotation after a security incident.",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := newSecretStore(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			// Generate a new key if none was provided.
			if newKey == "" {
				generated, err := secret.GenerateKey()
				if err != nil {
					return fmt.Errorf("generating new key: %w", err)
				}
				newKey = generated
			}

			newEncryptor, err := secret.NewEncryptor(newKey)
			if err != nil {
				return fmt.Errorf("invalid new key: %w", err)
			}

			count, err := store.ReEncryptAll(cmd.Context(), newEncryptor)
			if err != nil {
				return fmt.Errorf("key rotation failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Re-encrypted %d credential(s).\n", count)
			fmt.Fprintf(cmd.OutOrStdout(), "New key: %s\n", newKey)
			fmt.Fprintln(cmd.OutOrStdout(), "Update MANTLE_ENCRYPTION_KEY to the new key and restart.")

			return nil
		},
	}

	cmd.Flags().StringVar(&newKey, "new-key", "", "hex-encoded 32-byte new encryption key (auto-generated if omitted)")

	return cmd
}
