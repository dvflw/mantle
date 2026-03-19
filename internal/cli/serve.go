package cli

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/engine"
	"github.com/dvflw/mantle/internal/logging"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/dvflw/mantle/internal/server"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the Mantle server",
		Long:  "Starts a persistent process with the API server, cron scheduler, and webhook listener.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			// Configure structured JSON logging before any other work.
			logging.Setup(cfg.Log.Level)

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			// Run migrations on startup.
			if err := db.Migrate(cmd.Context(), database); err != nil {
				return fmt.Errorf("running migrations: %w", err)
			}

			eng, err := engine.New(database)
			if err != nil {
				return fmt.Errorf("creating engine: %w", err)
			}

			// Wire up Postgres-backed audit emitter.
			eng.Auditor = &audit.PostgresEmitter{DB: database}

			// Wire up credential resolver if encryption key is configured.
			if cfg.Encryption.Key != "" {
				encryptor, err := secret.NewEncryptor(cfg.Encryption.Key)
				if err != nil {
					return fmt.Errorf("configuring encryption: %w", err)
				}
				eng.Resolver = &secret.Resolver{
					Store: &secret.Store{DB: database, Encryptor: encryptor},
				}
			}

			srv := server.New(database, eng, cfg.API.Address)
			srv.AuthStore = &auth.Store{DB: database}

			// Handle SIGTERM and SIGINT for graceful shutdown.
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			fmt.Fprintf(cmd.OutOrStdout(), "Mantle server starting on %s\n", cfg.API.Address)
			return srv.Start(ctx)
		},
	}
}
