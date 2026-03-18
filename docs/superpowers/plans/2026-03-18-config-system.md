# Config System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Viper-based config system with `mantle.yaml`, env var overrides (`MANTLE_` prefix), and CLI flag overrides.

**Architecture:** `internal/config/` package owns the `Config` struct, `Load()` function, and context helpers. `Load()` receives the `*cobra.Command` to read `--config` and bind flags. Root command's `PersistentPreRunE` calls `Load()` and stores config on context.

**Tech Stack:** Go, Viper (`github.com/spf13/viper`), Cobra

**Spec:** `docs/superpowers/specs/2026-03-18-config-system-design.md`

**Linear issue:** [DVFLW-272](https://linear.app/dvflw/issue/DVFLW-272/config-system-mantleyaml-and-cli-flags)

---

### Task 1: Config struct and defaults

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Install Viper dependency**

Run:
```bash
go get github.com/spf13/viper
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
go test ./internal/config/ -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Write minimal implementation**

Create `internal/config/config.go`:

```go
package config

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds all engine configuration.
type Config struct {
	Database DatabaseConfig `mapstructure:"database"`
	API      APIConfig      `mapstructure:"api"`
	Log      LogConfig      `mapstructure:"log"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

// APIConfig holds API server settings.
type APIConfig struct {
	Address string `mapstructure:"address"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `mapstructure:"level"`
}

type contextKey struct{}

// WithContext returns a new context with the config attached.
func WithContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey{}, cfg)
}

// FromContext retrieves the config from context. Returns nil if not set.
func FromContext(ctx context.Context) *Config {
	cfg, _ := ctx.Value(contextKey{}).(*Config)
	return cfg
}

// Load reads configuration from file, env vars, and CLI flags.
// Precedence (highest to lowest): flags > env vars > config file > defaults.
func Load(cmd *cobra.Command) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("database.url", "postgres://localhost:5432/mantle?sslmode=disable")
	v.SetDefault("api.address", ":8080")
	v.SetDefault("log.level", "info")

	// Config file
	configPath, _ := cmd.Flags().GetString("config")
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("mantle")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		if configPath != "" {
			// Explicit --config path: hard error
			return nil, err
		}
		// No explicit path: ignore file-not-found, fail on parse errors
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	// Env vars — explicit binding for nested keys
	v.SetEnvPrefix("MANTLE")
	_ = v.BindEnv("database.url", "MANTLE_DATABASE_URL")
	_ = v.BindEnv("api.address", "MANTLE_API_ADDRESS")
	_ = v.BindEnv("log.level", "MANTLE_LOG_LEVEL")

	// CLI flag binding
	if f := cmd.Flags().Lookup("database-url"); f != nil {
		_ = v.BindPFlag("database.url", f)
	}
	if f := cmd.Flags().Lookup("api-address"); f != nil {
		_ = v.BindPFlag("api.address", f)
	}
	if f := cmd.Flags().Lookup("log-level"); f != nil {
		_ = v.BindPFlag("log.level", f)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
go test ./internal/config/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: add config package with Viper-based Load and defaults"
```

---

### Task 2: Config file loading

**Files:**
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test for config file loading**

Add to `internal/config/config_test.go`:

```go
import (
	"os"
	"path/filepath"
	// ... existing imports
)

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
	// Point config search to an empty temp dir to avoid picking up
	// a mantle.yaml from the test runner's working directory.
	// We use an explicit --config pointing to a non-yaml empty file
	// to skip implicit search. Instead, just use TestLoad_Defaults
	// which already covers the defaults path. This test verifies
	// that Load does NOT error when no config file is found implicitly.
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
```

- [ ] **Step 2: Run tests to verify they pass**

Run:
```bash
go test ./internal/config/ -v
```

Expected: PASS — all tests pass (config file loading, missing explicit config error, implicit missing uses defaults). If any fail, fix the implementation.

- [ ] **Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test: add config file loading and missing file behavior tests"
```

---

### Task 3: Environment variable overrides

**Files:**
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test for env var override**

Add to `internal/config/config_test.go`:

```go
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
	// api.address should still be default
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
```

- [ ] **Step 2: Run tests to verify they pass**

Run:
```bash
go test ./internal/config/ -v
```

Expected: PASS — env vars override defaults and config file values.

- [ ] **Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test: add env var override tests for config"
```

---

### Task 4: CLI flag overrides

**Files:**
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test for flag override**

Add to `internal/config/config_test.go`:

```go
func TestLoad_FlagOverridesAll(t *testing.T) {
	// Set env var — flag should still win
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
```

- [ ] **Step 2: Run tests to verify they pass**

Run:
```bash
go test ./internal/config/ -v
```

Expected: PASS — flags override env vars and config file.

- [ ] **Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test: add CLI flag override test for config"
```

---

### Task 5: Cobra integration — root command flags, PersistentPreRunE, and version bypass

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/version.go`
- Modify: `internal/cli/root_test.go`

Note: The `PersistentPreRunE` and the version command bypass MUST be added together. If we add `PersistentPreRunE` first, the existing `TestVersionCommand` would trigger config loading and could fail depending on the test environment. Adding both atomically avoids this.

- [ ] **Step 1: Write the failing tests**

Add to `internal/cli/root_test.go` (update imports to include `"strings"` and `"github.com/dvflw/mantle/internal/config"`):

```go
func TestRootCommand_ConfigLoaded(t *testing.T) {
	cmd := NewRootCommand()
	// Add a test subcommand that checks config is on context
	var gotConfig *config.Config
	cmd.AddCommand(&cobra.Command{
		Use: "testcfg",
		RunE: func(cmd *cobra.Command, args []string) error {
			gotConfig = config.FromContext(cmd.Context())
			return nil
		},
	})

	cmd.SetArgs([]string{"testcfg"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotConfig == nil {
		t.Fatal("expected config on context, got nil")
	}
	if gotConfig.API.Address != ":8080" {
		t.Errorf("API.Address = %q, want default :8080", gotConfig.API.Address)
	}
}

func TestRootCommand_ConfigFlagOverride(t *testing.T) {
	cmd := NewRootCommand()
	var gotConfig *config.Config
	cmd.AddCommand(&cobra.Command{
		Use: "testcfg",
		RunE: func(cmd *cobra.Command, args []string) error {
			gotConfig = config.FromContext(cmd.Context())
			return nil
		},
	})

	cmd.SetArgs([]string{"testcfg", "--log-level", "error"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotConfig == nil {
		t.Fatal("expected config on context, got nil")
	}
	if gotConfig.Log.Level != "error" {
		t.Errorf("Log.Level = %q, want error", gotConfig.Log.Level)
	}
}

func TestVersionCommand_WorksWithoutConfig(t *testing.T) {
	// Provide an explicit invalid config to prove version ignores it
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version", "--config", "/nonexistent/mantle.yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version should work even with invalid config, got error = %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "mantle ") {
		t.Errorf("expected version output, got %q", output)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/cli/ -v -run "TestRootCommand_Config|TestVersionCommand_Works"
```

Expected: FAIL — `PersistentPreRunE` not set, config not on context.

- [ ] **Step 3: Update root.go — add PersistentPreRunE and new flags**

Modify `internal/cli/root.go` to:

```go
package cli

import (
	"github.com/dvflw/mantle/internal/config"
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root mantle CLI command.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mantle",
		Short:        "Headless AI workflow automation platform",
		Long:         "Mantle is a headless AI workflow automation platform — BYOK, IaC-first, enterprise-grade, open source.",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cmd)
			if err != nil {
				return err
			}
			cmd.SetContext(config.WithContext(cmd.Context(), cfg))
			return nil
		},
	}

	cmd.PersistentFlags().String("config", "", "config file path (default: mantle.yaml)")
	cmd.PersistentFlags().String("database-url", "", "database connection URL")
	cmd.PersistentFlags().String("api-address", "", "API listen address")
	cmd.PersistentFlags().String("log-level", "", "log level (debug, info, warn, error)")

	cmd.AddCommand(newVersionCommand())

	return cmd
}
```

- [ ] **Step 4: Update version.go — add PersistentPreRunE bypass**

Modify `internal/cli/version.go` to add a no-op `PersistentPreRunE`:

```go
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil // Skip config loading
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
			return nil
		},
	}
}
```

- [ ] **Step 5: Run all cli tests to verify they pass**

Run:
```bash
go test ./internal/cli/ -v
```

Expected: PASS — all cli tests pass including config loading, flag override, and version bypass.

- [ ] **Step 6: Run all tests**

Run:
```bash
go test ./... -v
```

Expected: PASS — all tests across all packages pass.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/root.go internal/cli/version.go internal/cli/root_test.go
git commit -m "feat: integrate config loading into root command with version bypass"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run all tests**

Run:
```bash
go test ./... -v
```

Expected: All tests pass across `internal/config/`, `internal/cli/`, `internal/version/`.

- [ ] **Step 2: Run go mod tidy and vet**

Run:
```bash
go mod tidy && go vet ./...
```

Expected: No changes to go.mod/go.sum, no vet warnings.

- [ ] **Step 3: Build and verify**

Run:
```bash
make build && ./mantle version
```

Expected: Version output still works.

- [ ] **Step 4: Verify config flags in help**

Run:
```bash
./mantle --help
```

Expected: Shows `--config`, `--database-url`, `--api-address`, `--log-level` as global flags.

- [ ] **Step 5: Verify flag override works end-to-end**

Run:
```bash
./mantle version --log-level debug
```

Expected: Version prints normally (config doesn't affect version output, but flag parsing works without error).

- [ ] **Step 6: Clean up**

Run:
```bash
make clean
```
