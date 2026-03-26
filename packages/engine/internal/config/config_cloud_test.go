package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCloudConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(configFile, []byte(`
aws:
  region: "us-east-1"
gcp:
  region: "us-central1"
azure:
  region: "eastus"
`), 0644)
	require.NoError(t, err)

	cmd := newTestCommand()
	_ = cmd.Flags().Set("config", configFile)

	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "us-east-1", cfg.AWS.Region)
	assert.Equal(t, "us-central1", cfg.GCP.Region)
	assert.Equal(t, "eastus", cfg.Azure.Region)
}

func TestLoadCloudConfig_Empty(t *testing.T) {
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

	assert.Empty(t, cfg.AWS.Region)
	assert.Empty(t, cfg.GCP.Region)
	assert.Empty(t, cfg.Azure.Region)
}
