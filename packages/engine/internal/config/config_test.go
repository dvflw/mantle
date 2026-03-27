package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	if cfg.Database.URL != "postgres://mantle:mantle@localhost:5432/mantle?sslmode=prefer" {
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

	if cfg.Database.URL != "postgres://mantle:mantle@localhost:5432/mantle?sslmode=prefer" {
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

func TestConfig_EngineDefaults(t *testing.T) {
	cmd := newTestCommand()
	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, 200*time.Millisecond, cfg.Engine.WorkerPollInterval)
	assert.Equal(t, 5*time.Second, cfg.Engine.WorkerMaxBackoff)
	assert.Equal(t, 500*time.Millisecond, cfg.Engine.OrchestratorPollInterval)
	assert.Equal(t, 60*time.Second, cfg.Engine.StepLeaseDuration)
	assert.Equal(t, 120*time.Second, cfg.Engine.OrchestrationLeaseDuration)
	assert.Equal(t, 300*time.Second, cfg.Engine.AIStepLeaseDuration)
	assert.Equal(t, 30*time.Second, cfg.Engine.ReaperInterval)
	assert.Equal(t, 1048576, cfg.Engine.StepOutputMaxBytes)
	assert.Equal(t, 10, cfg.Engine.DefaultMaxToolRounds)
	assert.Equal(t, 10, cfg.Engine.DefaultMaxToolCallsPerRound)
	assert.NotEmpty(t, cfg.Engine.NodeID)
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

func TestLoad_BudgetDefaults(t *testing.T) {
	cmd := newTestCommand()

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "calendar", cfg.Engine.Budget.ResetMode)
	assert.Equal(t, 1, cfg.Engine.Budget.ResetDay)
	assert.Equal(t, int64(0), cfg.Engine.Budget.GlobalMonthlyTokenLimit)
	assert.Equal(t, int64(0), cfg.Engine.Budget.DefaultTeamMonthlyTokenLimit)
}

func TestLoad_BudgetResetDay_RollingInvalid(t *testing.T) {
	// ResetDay 0 with rolling mode should error
	t.Setenv("MANTLE_ENGINE_BUDGET_RESET_MODE", "rolling")
	t.Setenv("MANTLE_ENGINE_BUDGET_RESET_DAY", "0")

	cmd := newTestCommand()
	_, err := Load(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reset_day must be between 1 and 28")

	// ResetDay 29 with rolling mode should error
	t.Setenv("MANTLE_ENGINE_BUDGET_RESET_DAY", "29")
	cmd = newTestCommand()
	_, err = Load(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reset_day must be between 1 and 28")
}

func TestLoad_BudgetResetDay_CalendarClamps(t *testing.T) {
	// Invalid ResetDay with calendar mode should silently clamp to 1 (lower bound)
	t.Setenv("MANTLE_ENGINE_BUDGET_RESET_MODE", "calendar")
	t.Setenv("MANTLE_ENGINE_BUDGET_RESET_DAY", "0")

	cmd := newTestCommand()
	cfg, err := Load(cmd)
	require.NoError(t, err)
	assert.Equal(t, 1, cfg.Engine.Budget.ResetDay)
}

func TestLoad_BudgetResetDay_CalendarClampsUpperBound(t *testing.T) {
	// ResetDay >28 with calendar mode should silently clamp to 1
	// (calendar mode ignores reset_day entirely, so any invalid value is safe to clamp)
	t.Setenv("MANTLE_ENGINE_BUDGET_RESET_MODE", "calendar")
	t.Setenv("MANTLE_ENGINE_BUDGET_RESET_DAY", "100")

	cmd := newTestCommand()
	cfg, err := Load(cmd)
	require.NoError(t, err)
	assert.Equal(t, 1, cfg.Engine.Budget.ResetDay)
}

func TestLoad_TmpConfigFromEnvVars(t *testing.T) {
	t.Setenv("MANTLE_TMP_TYPE", "s3")
	t.Setenv("MANTLE_TMP_BUCKET", "my-artifacts")
	t.Setenv("MANTLE_TMP_PREFIX", "workflows/")
	t.Setenv("MANTLE_TMP_RETENTION", "24h")

	cmd := newTestCommand()
	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "s3", cfg.Tmp.Type)
	assert.Equal(t, "my-artifacts", cfg.Tmp.Bucket)
	assert.Equal(t, "workflows/", cfg.Tmp.Prefix)
	assert.Equal(t, "24h", cfg.Tmp.Retention)
}

func TestLoad_TmpConfigFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(configFile, []byte(`
tmp:
  type: "filesystem"
  path: "/tmp/mantle-artifacts"
  retention: "48h"
`), 0644)
	require.NoError(t, err)

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "filesystem", cfg.Tmp.Type)
	assert.Equal(t, "/tmp/mantle-artifacts", cfg.Tmp.Path)
	assert.Equal(t, "48h", cfg.Tmp.Retention)
}

func TestLoad_TmpConfigEnvVarOverridesFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	_ = os.WriteFile(configFile, []byte(`
tmp:
  type: "filesystem"
  path: "/tmp/file-path"
`), 0644)

	t.Setenv("MANTLE_TMP_TYPE", "s3")
	t.Setenv("MANTLE_TMP_BUCKET", "env-bucket")

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "s3", cfg.Tmp.Type)
	assert.Equal(t, "env-bucket", cfg.Tmp.Bucket)
	// path from file should still be present since env var didn't override it
	assert.Equal(t, "/tmp/file-path", cfg.Tmp.Path)
}

func TestLoad_EnvMap(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(configFile, []byte(`
env:
  APP_NAME: "my-app"
  REGION: "us-east-1"
  DEBUG: "true"
`), 0644)
	require.NoError(t, err)

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "my-app", cfg.Env["APP_NAME"])
	assert.Equal(t, "us-east-1", cfg.Env["REGION"])
	assert.Equal(t, "true", cfg.Env["DEBUG"])
}

func TestLoad_EnvMapEmpty(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(configFile, []byte(`
log:
  level: "debug"
`), 0644)
	require.NoError(t, err)

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Empty(t, cfg.Env)
}
