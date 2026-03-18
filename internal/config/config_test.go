package config

import (
	"testing"

	"github.com/spf13/cobra"
)

// newTestCommand creates a cobra.Command with all config-related flags registered.
func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("api-address", "", "")
	cmd.Flags().String("log-level", "", "")
	return cmd
}

func TestLoad_Defaults(t *testing.T) {
	cmd := newTestCommand()

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.URL != "postgres://localhost:5432/mantle?sslmode=disable" {
		t.Errorf("Database.URL = %q, want default", cfg.Database.URL)
	}
	if cfg.API.Address != ":8080" {
		t.Errorf("API.Address = %q, want :8080", cfg.API.Address)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want info", cfg.Log.Level)
	}
}
