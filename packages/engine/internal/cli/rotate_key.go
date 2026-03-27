package cli

import (
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
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
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			if cfg.Encryption.Key == "" {
				return fmt.Errorf("encryption key not configured (set MANTLE_ENCRYPTION_KEY or encryption.key in mantle.yaml)")
			}

			oldEncryptor, err := secret.NewEncryptor(cfg.Encryption.Key)
			if err != nil {
				return fmt.Errorf("invalid current key: %w", err)
			}

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

			database, err := db.Open(cfg.Database)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			tx, err := database.BeginTx(cmd.Context(), nil)
			if err != nil {
				return fmt.Errorf("starting transaction: %w", err)
			}
			defer tx.Rollback()

			count, err := secret.RotateAll(cmd.Context(), tx, oldEncryptor, newEncryptor)
			if err != nil {
				return fmt.Errorf("key rotation failed: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("committing transaction: %w", err)
			}

			// Emit audit event after successful rotation.
			auditor := &audit.PostgresEmitter{DB: database}
			_ = auditor.Emit(cmd.Context(), audit.Event{
				Timestamp: time.Now(),
				Actor:     "cli",
				Action:    audit.ActionSecretKeyRotated,
				Resource:  audit.Resource{Type: "credentials", ID: "all"},
				Metadata:  map[string]string{"count": fmt.Sprintf("%d", count)},
			})

			fmt.Fprintf(cmd.OutOrStdout(), "Re-encrypted %d credential(s).\n", count)
			fmt.Fprintf(cmd.OutOrStdout(), "New key: %s\n", newKey)
			fmt.Fprintln(cmd.OutOrStdout(), "Update MANTLE_ENCRYPTION_KEY to the new key and restart.")

			return nil
		},
	}

	cmd.Flags().StringVar(&newKey, "new-key", "", "hex-encoded 32-byte new encryption key (auto-generated if omitted)")

	return cmd
}
