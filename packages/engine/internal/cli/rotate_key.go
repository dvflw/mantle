package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/spf13/cobra"
)

func newSecretsRotateKeyCommand() *cobra.Command {
	var newKeyFile string
	var outputKey string

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

			// Resolve the new key: from file, or auto-generate.
			var newKey string
			userProvided := false
			if newKeyFile != "" {
				raw, err := os.ReadFile(newKeyFile)
				if err != nil {
					return fmt.Errorf("reading new key file: %w", err)
				}
				newKey = strings.TrimSpace(string(raw))
				userProvided = true
			} else {
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

			// Emit audit event inside the transaction so it commits atomically.
			// Use context.Background() to prevent cancellation from dropping the audit record.
			if err := audit.EmitTx(context.Background(), tx, audit.Event{
				Timestamp: time.Now(),
				Actor:     "cli",
				Action:    audit.ActionSecretKeyRotated,
				Resource:  audit.Resource{Type: "credentials", ID: "all"},
				Metadata:  map[string]string{"count": fmt.Sprintf("%d", count)},
			}); err != nil {
				return fmt.Errorf("emitting audit event: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("committing transaction: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Re-encrypted %d credential(s).\n", count)

			// Output the new key securely.
			if !userProvided {
				if outputKey != "" {
					if err := os.WriteFile(outputKey, []byte(newKey+"\n"), 0600); err != nil {
						return fmt.Errorf("writing key to file: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "New key written to: %s\n", outputKey)
				} else {
					fmt.Fprintln(cmd.ErrOrStderr(), "WARNING: The following key is sensitive. Store it securely and clear your terminal.")
					fmt.Fprintf(cmd.OutOrStdout(), "New key: %s\n", newKey)
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Update MANTLE_ENCRYPTION_KEY to the new key and restart.")

			return nil
		},
	}

	cmd.Flags().StringVar(&newKeyFile, "new-key-file", "", "path to file containing hex-encoded 32-byte new encryption key (auto-generated if omitted)")
	cmd.Flags().StringVar(&outputKey, "output-key", "", "path to write the auto-generated key (file permissions: 0600)")

	return cmd
}
