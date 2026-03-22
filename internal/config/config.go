package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds all engine configuration.
type Config struct {
	Database   DatabaseConfig   `mapstructure:"database"`
	API        APIConfig        `mapstructure:"api"`
	Log        LogConfig        `mapstructure:"log"`
	Encryption EncryptionConfig `mapstructure:"encryption"`
	Engine     EngineConfig     `mapstructure:"engine"`
	Auth       AuthConfig       `mapstructure:"auth"`
	AWS        AWSConfig        `mapstructure:"aws"`
	GCP        GCPConfig        `mapstructure:"gcp"`
	Azure      AzureConfig      `mapstructure:"azure"`
}

// AWSConfig holds AWS provider settings.
type AWSConfig struct {
	Region string `mapstructure:"region"`
}

// GCPConfig holds GCP provider settings.
type GCPConfig struct {
	Region string `mapstructure:"region"`
}

// AzureConfig holds Azure provider settings.
type AzureConfig struct {
	Region string `mapstructure:"region"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	OIDC OIDCConfig `mapstructure:"oidc"`
}

// OIDCConfig holds OIDC provider settings.
type OIDCConfig struct {
	IssuerURL      string   `mapstructure:"issuer_url"`
	ClientID       string   `mapstructure:"client_id"`
	ClientSecret   string   `mapstructure:"client_secret"`
	Audience       string   `mapstructure:"audience"`
	AllowedDomains []string `mapstructure:"allowed_domains"`
}

// EncryptionConfig holds the master encryption key for credential storage.
type EncryptionConfig struct {
	Key string `mapstructure:"key"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	URL             string        `mapstructure:"url"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// TLSConfig holds TLS certificate settings.
type TLSConfig struct {
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// APIConfig holds API server settings.
type APIConfig struct {
	Address string    `mapstructure:"address"`
	TLS     TLSConfig `mapstructure:"tls"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `mapstructure:"level"`
}

// EngineConfig holds distributed engine settings.
type EngineConfig struct {
	NodeID                      string        `mapstructure:"node_id"`
	WorkerPollInterval          time.Duration `mapstructure:"worker_poll_interval"`
	WorkerMaxBackoff            time.Duration `mapstructure:"worker_max_backoff"`
	OrchestratorPollInterval    time.Duration `mapstructure:"orchestrator_poll_interval"`
	StepLeaseDuration           time.Duration `mapstructure:"step_lease_duration"`
	OrchestrationLeaseDuration  time.Duration `mapstructure:"orchestration_lease_duration"`
	AIStepLeaseDuration         time.Duration `mapstructure:"ai_step_lease_duration"`
	ReaperInterval              time.Duration `mapstructure:"reaper_interval"`
	StepOutputMaxBytes          int           `mapstructure:"step_output_max_bytes"`
	DefaultMaxToolRounds        int           `mapstructure:"default_max_tool_rounds"`
	DefaultMaxToolCallsPerRound int           `mapstructure:"default_max_tool_calls_per_round"`
	AllowedBaseURLs             []string      `mapstructure:"allowed_base_urls"`
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
	v.SetDefault("database.url", "postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 25)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)
	v.SetDefault("api.address", ":8080")
	v.SetDefault("log.level", "info")

	// Engine defaults
	v.SetDefault("engine.worker_poll_interval", 200*time.Millisecond)
	v.SetDefault("engine.worker_max_backoff", 5*time.Second)
	v.SetDefault("engine.orchestrator_poll_interval", 500*time.Millisecond)
	v.SetDefault("engine.step_lease_duration", 60*time.Second)
	v.SetDefault("engine.orchestration_lease_duration", 120*time.Second)
	v.SetDefault("engine.ai_step_lease_duration", 300*time.Second)
	v.SetDefault("engine.reaper_interval", 30*time.Second)
	v.SetDefault("engine.step_output_max_bytes", 1048576)
	v.SetDefault("engine.default_max_tool_rounds", 10)
	v.SetDefault("engine.default_max_tool_calls_per_round", 10)

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
		// No explicit path: silently ignore all errors.
		// Viper may find non-config files matching the name (e.g., the mantle binary)
		// and fail to parse them. Since no config file was explicitly requested,
		// falling back to defaults is always safe.
	}

	// Env vars — explicit binding for nested keys
	v.SetEnvPrefix("MANTLE")
	_ = v.BindEnv("database.url", "MANTLE_DATABASE_URL")
	_ = v.BindEnv("api.address", "MANTLE_API_ADDRESS")
	_ = v.BindEnv("log.level", "MANTLE_LOG_LEVEL")
	_ = v.BindEnv("encryption.key", "MANTLE_ENCRYPTION_KEY")
	_ = v.BindEnv("database.max_open_conns", "MANTLE_DATABASE_MAX_OPEN_CONNS")
	_ = v.BindEnv("database.max_idle_conns", "MANTLE_DATABASE_MAX_IDLE_CONNS")
	_ = v.BindEnv("database.conn_max_lifetime", "MANTLE_DATABASE_CONN_MAX_LIFETIME")

	// Auth/OIDC env var bindings
	_ = v.BindEnv("auth.oidc.issuer_url", "MANTLE_AUTH_OIDC_ISSUER_URL")
	_ = v.BindEnv("auth.oidc.client_id", "MANTLE_AUTH_OIDC_CLIENT_ID")
	_ = v.BindEnv("auth.oidc.client_secret", "MANTLE_AUTH_OIDC_CLIENT_SECRET")
	_ = v.BindEnv("auth.oidc.audience", "MANTLE_AUTH_OIDC_AUDIENCE")
	_ = v.BindEnv("auth.oidc.allowed_domains", "MANTLE_AUTH_OIDC_ALLOWED_DOMAINS")

	// TLS env var bindings
	_ = v.BindEnv("api.tls.cert_file", "MANTLE_API_TLS_CERT_FILE")
	_ = v.BindEnv("api.tls.key_file", "MANTLE_API_TLS_KEY_FILE")

	// Cloud provider env var bindings
	_ = v.BindEnv("aws.region", "MANTLE_AWS_REGION")
	_ = v.BindEnv("gcp.region", "MANTLE_GCP_REGION")
	_ = v.BindEnv("azure.region", "MANTLE_AZURE_REGION")

	// Engine env var bindings
	_ = v.BindEnv("engine.node_id", "MANTLE_ENGINE_NODE_ID")
	_ = v.BindEnv("engine.worker_poll_interval", "MANTLE_ENGINE_WORKER_POLL_INTERVAL")
	_ = v.BindEnv("engine.worker_max_backoff", "MANTLE_ENGINE_WORKER_MAX_BACKOFF")
	_ = v.BindEnv("engine.orchestrator_poll_interval", "MANTLE_ENGINE_ORCHESTRATOR_POLL_INTERVAL")
	_ = v.BindEnv("engine.step_lease_duration", "MANTLE_ENGINE_STEP_LEASE_DURATION")
	_ = v.BindEnv("engine.orchestration_lease_duration", "MANTLE_ENGINE_ORCHESTRATION_LEASE_DURATION")
	_ = v.BindEnv("engine.ai_step_lease_duration", "MANTLE_ENGINE_AI_STEP_LEASE_DURATION")
	_ = v.BindEnv("engine.reaper_interval", "MANTLE_ENGINE_REAPER_INTERVAL")
	_ = v.BindEnv("engine.step_output_max_bytes", "MANTLE_ENGINE_STEP_OUTPUT_MAX_BYTES")
	_ = v.BindEnv("engine.default_max_tool_rounds", "MANTLE_ENGINE_DEFAULT_MAX_TOOL_ROUNDS")
	_ = v.BindEnv("engine.default_max_tool_calls_per_round", "MANTLE_ENGINE_DEFAULT_MAX_TOOL_CALLS_PER_ROUND")
	_ = v.BindEnv("engine.allowed_base_urls", "MANTLE_ENGINE_ALLOWED_BASE_URLS")

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

	// Generate default NodeID if not set.
	// Format: hostname:pid:random8chars — the random suffix ensures uniqueness
	// across Kubernetes container restarts where PID 1 is common.
	if cfg.Engine.NodeID == "" {
		hostname, _ := os.Hostname()
		var suffix [4]byte
		_, _ = rand.Read(suffix[:])
		cfg.Engine.NodeID = fmt.Sprintf("%s:%d:%s", hostname, os.Getpid(), hex.EncodeToString(suffix[:]))
	}

	return &cfg, nil
}
