package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/budget"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/connector"
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

			database, err := db.Open(cfg.Database)
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
			eng.MaxToolRoundsLimit = cfg.Engine.MaxToolRoundsLimit

			// Configure AWS defaults for AI (Bedrock) and S3 connectors.
			if aiConn, err := eng.Registry.Get("ai/completion"); err == nil {
				if ai, ok := aiConn.(*connector.AIConnector); ok {
					if cfg.AWS.Region != "" {
						ai.DefaultRegion = cfg.AWS.Region
						ai.AWSConfigFunc = connector.NewAWSConfig
					}
					ai.AllowedBaseURLs = cfg.Engine.AllowedBaseURLs
					ai.AllowedModels = cfg.Engine.AllowedModels
				}
			}

			// Wire up Postgres-backed audit emitter with auth context enrichment.
			auditor := &audit.PostgresEmitter{
				DB:                  database,
				AuthMethodExtractor: auth.AuthMethodFromContext,
			}
			eng.Auditor = auditor

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

			// Wire budget enforcement into the engine.
			budgetStore := budget.NewStore(database)
			eng.BudgetStore = budgetStore
			eng.BudgetChecker = &budget.Checker{
				GlobalMonthlyTokenLimit:      cfg.Engine.Budget.GlobalMonthlyTokenLimit,
				DefaultTeamMonthlyTokenLimit: cfg.Engine.Budget.DefaultTeamMonthlyTokenLimit,
				ResetMode:                    cfg.Engine.Budget.ResetMode,
				ResetDay:                     cfg.Engine.Budget.ResetDay,
				GetTotalUsage: func(ctx context.Context, period time.Time) (int64, error) {
					u, err := budgetStore.GetGlobalTotalUsage(ctx, period)
					if err != nil {
						return 0, err
					}
					return u.TotalTokens, nil
				},
				GetProviderUsage: func(ctx context.Context, teamID, provider string, period time.Time) (int64, error) {
					u, err := budgetStore.GetUsage(ctx, teamID, provider, period)
					if err != nil {
						return 0, err
					}
					return u.TotalTokens, nil
				},
				GetTeamBudget: func(ctx context.Context, teamID, provider string) (*budget.TeamBudget, error) {
					return budgetStore.GetTeamBudget(ctx, teamID, provider)
				},
			}

			srv := server.New(database, eng, cfg.API.Address)
			srv.Auditor = auditor
			srv.TLSCertFile = cfg.API.TLS.CertFile
			srv.TLSKeyFile = cfg.API.TLS.KeyFile
			srv.AuthStore = &auth.Store{DB: database}
			srv.BudgetStore = budgetStore

			// Wire up OIDC validator if configured.
			if cfg.Auth.OIDC.IssuerURL != "" {
				oidcValidator, err := auth.NewOIDCValidator(
					cmd.Context(),
					cfg.Auth.OIDC.IssuerURL,
					cfg.Auth.OIDC.ClientID,
					cfg.Auth.OIDC.Audience,
					cfg.Auth.OIDC.AllowedDomains,
				)
				if err != nil {
					return fmt.Errorf("configuring OIDC: %w", err)
				}
				srv.OIDCValidator = oidcValidator
				fmt.Fprintf(cmd.OutOrStdout(), "OIDC authentication enabled (issuer: %s)\n", cfg.Auth.OIDC.IssuerURL)
			}

			// Handle SIGTERM and SIGINT for graceful shutdown.
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			fmt.Fprintf(cmd.OutOrStdout(), "Mantle server starting on %s\n", cfg.API.Address)
			return srv.Start(ctx)
		},
	}
}
