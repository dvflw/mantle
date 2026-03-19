package config

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds all engine configuration.
type Config struct {
	Database   DatabaseConfig   `mapstructure:"database"`
	API        APIConfig        `mapstructure:"api"`
	Log        LogConfig        `mapstructure:"log"`
	Encryption EncryptionConfig `mapstructure:"encryption"`
}

// EncryptionConfig holds the master encryption key for credential storage.
type EncryptionConfig struct {
	Key string `mapstructure:"key"`
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
	v.SetDefault("database.url", "postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable")
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
