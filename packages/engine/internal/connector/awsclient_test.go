package connector

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAWSConfig_ExplicitCredentials(t *testing.T) {
	// Unset any ambient AWS env vars so static creds are the sole source.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_REGION", "")

	cred := map[string]string{
		"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
		"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"region":            "us-east-1",
	}

	cfg, err := NewAWSConfig(context.Background(), cred, "")
	require.NoError(t, err)

	assert.Equal(t, "us-east-1", cfg.Region)

	// Retrieve credentials and verify they match what we passed in.
	retrieved, err := cfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", retrieved.AccessKeyID)
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", retrieved.SecretAccessKey)
	assert.Equal(t, "", retrieved.SessionToken)
}

func TestNewAWSConfig_DefaultRegionFallback(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_REGION", "")

	// Credential map has no region key — should fall back to defaultRegion parameter.
	cred := map[string]string{
		"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
		"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	cfg, err := NewAWSConfig(context.Background(), cred, "eu-west-1")
	require.NoError(t, err)

	assert.Equal(t, "eu-west-1", cfg.Region)
}

func TestNewAWSConfig_SessionToken(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_REGION", "")

	cred := map[string]string{
		"access_key_id":     "ASIAIOSFODNN7EXAMPLE",
		"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"session_token":     "AQoDYXdzEJr//some+session+token==",
		"region":            "ap-southeast-1",
	}

	cfg, err := NewAWSConfig(context.Background(), cred, "")
	require.NoError(t, err)

	retrieved, err := cfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ASIAIOSFODNN7EXAMPLE", retrieved.AccessKeyID)
	assert.Equal(t, "AQoDYXdzEJr//some+session+token==", retrieved.SessionToken)
	assert.Equal(t, "ap-southeast-1", cfg.Region)
}

func TestNewAWSConfig_NilCredential_UsesDefaultChain(t *testing.T) {
	// Provide credentials and region via environment (SDK default chain).
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAENV0000000000000")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "envSecretKey")
	t.Setenv("AWS_REGION", "us-west-2")

	cfg, err := NewAWSConfig(context.Background(), nil, "")
	require.NoError(t, err)

	assert.Equal(t, "us-west-2", cfg.Region)

	retrieved, err := cfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "AKIAENV0000000000000", retrieved.AccessKeyID)
}

func TestNewAWSConfig_MissingRegion(t *testing.T) {
	// No credential, no env region, no defaultRegion — must return an error.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	_, err := NewAWSConfig(context.Background(), nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS region is required")
}
