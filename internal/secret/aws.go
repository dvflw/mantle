package secret

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// SecretsManagerAPI is the subset of the AWS Secrets Manager client needed by
// the backend. It exists so callers can supply a mock in tests.
type SecretsManagerAPI interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// AWSSecretsManagerBackend resolves credentials from AWS Secrets Manager.
// The secret name in AWS maps 1:1 to the credential name in Mantle.
type AWSSecretsManagerBackend struct {
	client SecretsManagerAPI
}

// NewAWSSecretsManagerBackend creates a backend using the default AWS credential
// chain (env vars, shared config, instance profile, etc.) in the given region.
func NewAWSSecretsManagerBackend(ctx context.Context, region string) (*AWSSecretsManagerBackend, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	client := secretsmanager.NewFromConfig(cfg)
	return &AWSSecretsManagerBackend{client: client}, nil
}

// NewAWSSecretsManagerBackendFromClient creates a backend from a pre-built
// client. This is the primary constructor for tests.
func NewAWSSecretsManagerBackendFromClient(client SecretsManagerAPI) *AWSSecretsManagerBackend {
	return &AWSSecretsManagerBackend{client: client}
}

// Resolve fetches a secret by name from AWS Secrets Manager.
// If the secret value is valid JSON, it is decoded into map[string]string.
// Otherwise the raw string is returned as {"key": "<value>"}.
func (b *AWSSecretsManagerBackend) Resolve(ctx context.Context, name string) (map[string]string, error) {
	out, err := b.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return nil, fmt.Errorf("fetching secret %q from AWS Secrets Manager: %w", name, err)
	}

	if out.SecretString == nil {
		return nil, fmt.Errorf("secret %q has no string value (binary secrets are not supported)", name)
	}

	raw := *out.SecretString

	// Attempt JSON parse first.
	var data map[string]string
	if err := json.Unmarshal([]byte(raw), &data); err == nil {
		return data, nil
	}

	// Fallback: treat the entire value as a single key.
	return map[string]string{"key": raw}, nil
}
