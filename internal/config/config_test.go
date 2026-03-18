package config

import (
	"os"
	"path/filepath"
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

func TestLoad_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(configFile, []byte(`
database:
  url: "postgres://custom:5432/mydb"
api:
  address: ":9090"
log:
  level: "debug"
`), 0644)
	if err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.URL != "postgres://custom:5432/mydb" {
		t.Errorf("Database.URL = %q, want postgres://custom:5432/mydb", cfg.Database.URL)
	}
	if cfg.API.Address != ":9090" {
		t.Errorf("API.Address = %q, want :9090", cfg.API.Address)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug", cfg.Log.Level)
	}
}

func TestLoad_ExplicitConfigNotFound(t *testing.T) {
	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", "/nonexistent/mantle.yaml")

	_, err := Load(cmd)
	if err == nil {
		t.Fatal("Load() expected error for missing explicit config, got nil")
	}
}

func TestLoad_ImplicitConfigMissing_UsesDefaults(t *testing.T) {
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(origDir) }()

	cmd := newTestCommand()

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil (silent fallback)", err)
	}

	if cfg.Database.URL != "postgres://localhost:5432/mantle?sslmode=disable" {
		t.Errorf("Database.URL = %q, want default", cfg.Database.URL)
	}
}

func TestLoad_EnvVarOverridesDefault(t *testing.T) {
	t.Setenv("MANTLE_DATABASE_URL", "postgres://envhost:5432/envdb")
	t.Setenv("MANTLE_LOG_LEVEL", "warn")

	cmd := newTestCommand()

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.URL != "postgres://envhost:5432/envdb" {
		t.Errorf("Database.URL = %q, want env override", cfg.Database.URL)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want warn", cfg.Log.Level)
	}
	if cfg.API.Address != ":8080" {
		t.Errorf("API.Address = %q, want default :8080", cfg.API.Address)
	}
}

func TestLoad_EnvVarOverridesConfigFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	_ = os.WriteFile(configFile, []byte(`
database:
  url: "postgres://filehost:5432/filedb"
`), 0644)

	t.Setenv("MANTLE_DATABASE_URL", "postgres://envhost:5432/envdb")

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.URL != "postgres://envhost:5432/envdb" {
		t.Errorf("Database.URL = %q, want env override over file", cfg.Database.URL)
	}
}

func TestLoad_FlagOverridesAll(t *testing.T) {
	t.Setenv("MANTLE_DATABASE_URL", "postgres://envhost:5432/envdb")

	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	_ = os.WriteFile(configFile, []byte(`
database:
  url: "postgres://filehost:5432/filedb"
log:
  level: "debug"
`), 0644)

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)
	_ = cmd.Flags().Set("database-url", "postgres://flaghost:5432/flagdb")
	_ = cmd.Flags().Set("log-level", "error")

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Database.URL != "postgres://flaghost:5432/flagdb" {
		t.Errorf("Database.URL = %q, want flag override", cfg.Database.URL)
	}
	if cfg.Log.Level != "error" {
		t.Errorf("Log.Level = %q, want error (flag override)", cfg.Log.Level)
	}
}
