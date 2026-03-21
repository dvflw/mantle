package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadOIDCConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(configFile, []byte(`
auth:
  oidc:
    issuer_url: "https://accounts.example.com"
    client_id: "myclientid"
    client_secret: "mysecret"
    audience: "myaudience"
    allowed_domains:
      - "example.com"
      - "corp.example.com"
`), 0644)
	require.NoError(t, err)

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "https://accounts.example.com", cfg.Auth.OIDC.IssuerURL)
	assert.Equal(t, "myclientid", cfg.Auth.OIDC.ClientID)
	assert.Equal(t, "mysecret", cfg.Auth.OIDC.ClientSecret)
	assert.Equal(t, "myaudience", cfg.Auth.OIDC.Audience)
	assert.Equal(t, []string{"example.com", "corp.example.com"}, cfg.Auth.OIDC.AllowedDomains)
}

func TestLoadOIDCConfig_Empty(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(configFile, []byte(`
database:
  url: "postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable"
`), 0644)
	require.NoError(t, err)

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Empty(t, cfg.Auth.OIDC.IssuerURL)
	assert.Empty(t, cfg.Auth.OIDC.ClientID)
	assert.Empty(t, cfg.Auth.OIDC.ClientSecret)
	assert.Empty(t, cfg.Auth.OIDC.Audience)
	assert.Empty(t, cfg.Auth.OIDC.AllowedDomains)
}
