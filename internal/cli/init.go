package cli

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/netutil"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize Mantle — run database migrations",
		Long:  "Runs all pending database migrations to set up or upgrade the Mantle schema.\nIf Postgres is not reachable, offers to start one automatically via Docker.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				database, err = handleConnectionFailure(cmd, cfg, err)
				if err != nil {
					return err
				}
			}
			defer database.Close()

			fmt.Fprintln(cmd.OutOrStdout(), "Running migrations...")
			if err := db.Migrate(cmd.Context(), database); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Migrations complete.")
			return nil
		},
	}
}

// handleConnectionFailure is called when the initial db.Open fails.
// It classifies the host and offers interactive recovery options.
func handleConnectionFailure(cmd *cobra.Command, cfg *config.Config, connErr error) (*sql.DB, error) {
	host := parseHostFromURL(cfg.Database.URL)

	// Non-interactive mode (piped stdin, CI): just return the error.
	if !isInteractive() {
		return nil, fmt.Errorf("failed to connect to database: %w", connErr)
	}

	if netutil.IsLoopback(host) {
		return handleLoopbackFailure(cmd, cfg, connErr)
	}
	return handleRemoteFailure(cmd, cfg, host, connErr)
}


// handleLoopbackFailure offers Docker auto-provisioning for localhost connections.
func handleLoopbackFailure(cmd *cobra.Command, cfg *config.Config, connErr error) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	fmt.Fprintf(out, "No Postgres found on localhost: %v\n\n", connErr)
	fmt.Fprint(out, "Start a Postgres container with Docker? [Y/n]: ")

	var answer string
	fmt.Fscanln(in, &answer)
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "" && answer != "y" && answer != "yes" {
		return promptConnectionStringOrRetryDocker(cmd, cfg)
	}

	// User accepted Docker provisioning.
	return attemptDockerProvisioning(cmd, cfg)
}

// attemptDockerProvisioning checks Docker availability and starts the container.
func attemptDockerProvisioning(cmd *cobra.Command, cfg *config.Config) (*sql.DB, error) {
	out := cmd.OutOrStdout()

	if !dockerAvailable() {
		fmt.Fprintln(out, "\nDocker isn't installed or isn't running.")
		return promptConnectionStringOrRetryDocker(cmd, cfg)
	}

	fmt.Fprintln(out, "Starting Postgres container...")
	if err := dockerStartPostgres(cfg.Database); err != nil {
		return nil, fmt.Errorf("docker provisioning failed: %w", err)
	}

	fmt.Fprintln(out, "Postgres is ready.")
	return db.Open(cfg.Database)
}

// promptConnectionStringOrRetryDocker offers [R]etry or [C]onnection string.
func promptConnectionStringOrRetryDocker(cmd *cobra.Command, cfg *config.Config) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	for {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  [R] Retry (install or start Docker first)")
		fmt.Fprintln(out, "  [C] Enter a Postgres connection string")
		fmt.Fprint(out, "\nChoice [R/c]: ")

		var choice string
		fmt.Fscanln(in, &choice)
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "c":
			return promptConnectionString(cmd, cfg)
		default:
			// Retry Docker provisioning.
			return attemptDockerProvisioning(cmd, cfg)
		}
	}
}

// promptConnectionString asks the user for a connection URL and validates it.
func promptConnectionString(cmd *cobra.Command, cfg *config.Config) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	for {
		fmt.Fprint(out, "Postgres connection string: ")

		var connStr string
		fmt.Fscanln(in, &connStr)
		connStr = strings.TrimSpace(connStr)

		if connStr == "" {
			continue
		}

		cfg.Database.URL = connStr
		database, err := db.Open(cfg.Database)
		if err != nil {
			fmt.Fprintf(out, "Connection failed: %v\n", err)
			continue
		}
		return database, nil
	}
}

// handleRemoteFailure shows the error and offers retry/quit for non-loopback hosts.
func handleRemoteFailure(cmd *cobra.Command, cfg *config.Config, host string, connErr error) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	for {
		fmt.Fprintf(out, "Failed to connect to database at %s\n\n", host)
		fmt.Fprintf(out, "  Error: %v\n\n", connErr)
		fmt.Fprintln(out, "  [R] Retry (fix the issue and try again)")
		fmt.Fprintln(out, "  [Q] Quit")
		fmt.Fprint(out, "\nChoice [R/q]: ")

		var choice string
		fmt.Fscanln(in, &choice)
		choice = strings.TrimSpace(strings.ToLower(choice))

		if choice == "q" {
			return nil, fmt.Errorf("failed to connect to database at %s: %w", host, connErr)
		}

		// Retry: re-load config to pick up env var / config file changes.
		newCfg, err := config.Load(cmd)
		if err != nil {
			fmt.Fprintf(out, "Config reload error: %v\n", err)
			continue
		}
		cfg.Database = newCfg.Database

		database, err := db.Open(cfg.Database)
		if err != nil {
			connErr = err
			host = parseHostFromURL(cfg.Database.URL)
			continue
		}
		return database, nil
	}
}
