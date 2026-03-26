package secret

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// mockAWSSMClient implements SecretsManagerAPI for testing.
type mockAWSSMClient struct {
	secrets map[string]*secretsmanager.GetSecretValueOutput
}

func (m *mockAWSSMClient) GetSecretValue(
	ctx context.Context,
	params *secretsmanager.GetSecretValueInput,
	optFns ...func(*secretsmanager.Options),
) (*secretsmanager.GetSecretValueOutput, error) {
	name := aws.ToString(params.SecretId)
	out, ok := m.secrets[name]
	if !ok {
		return nil, fmt.Errorf("ResourceNotFoundException: secret %q not found", name)
	}
	return out, nil
}

func TestAWSBackend_JSONSecret(t *testing.T) {
	client := &mockAWSSMClient{
		secrets: map[string]*secretsmanager.GetSecretValueOutput{
			"my-api-cred": {
				SecretString: aws.String(`{"api_key":"sk-abc123","org_id":"org-xyz"}`),
			},
		},
	}
	backend := NewAWSSecretsManagerBackendFromClient(client)

	data, err := backend.Resolve(context.Background(), "my-api-cred")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["api_key"] != "sk-abc123" {
		t.Errorf("api_key = %q, want %q", data["api_key"], "sk-abc123")
	}
	if data["org_id"] != "org-xyz" {
		t.Errorf("org_id = %q, want %q", data["org_id"], "org-xyz")
	}
}

func TestAWSBackend_PlainStringFallback(t *testing.T) {
	client := &mockAWSSMClient{
		secrets: map[string]*secretsmanager.GetSecretValueOutput{
			"simple-token": {
				SecretString: aws.String("my-raw-token-value"),
			},
		},
	}
	backend := NewAWSSecretsManagerBackendFromClient(client)

	data, err := backend.Resolve(context.Background(), "simple-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["key"] != "my-raw-token-value" {
		t.Errorf("key = %q, want %q", data["key"], "my-raw-token-value")
	}
	if len(data) != 1 {
		t.Errorf("expected 1 field, got %d", len(data))
	}
}

func TestAWSBackend_SecretNotFound(t *testing.T) {
	client := &mockAWSSMClient{
		secrets: map[string]*secretsmanager.GetSecretValueOutput{},
	}
	backend := NewAWSSecretsManagerBackendFromClient(client)

	_, err := backend.Resolve(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}
}

func TestAWSBackend_BinarySecretReturnsError(t *testing.T) {
	client := &mockAWSSMClient{
		secrets: map[string]*secretsmanager.GetSecretValueOutput{
			"binary-secret": {
				SecretBinary: []byte("binary-data"),
				// SecretString is nil
			},
		},
	}
	backend := NewAWSSecretsManagerBackendFromClient(client)

	_, err := backend.Resolve(context.Background(), "binary-secret")
	if err == nil {
		t.Fatal("expected error for binary secret, got nil")
	}
}

func TestAWSBackend_SecretNameMapping(t *testing.T) {
	// Verify that the Mantle credential name is passed directly as the AWS
	// secret ID (1:1 mapping, no transformation).
	client := &mockAWSSMClient{
		secrets: map[string]*secretsmanager.GetSecretValueOutput{
			"prod/my-service/db-creds": {
				SecretString: aws.String(`{"username":"admin","password":"s3cret"}`),
			},
		},
	}

	backend := NewAWSSecretsManagerBackendFromClient(client)
	data, err := backend.Resolve(context.Background(), "prod/my-service/db-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["username"] != "admin" {
		t.Errorf("username = %q, want %q", data["username"], "admin")
	}
	if data["password"] != "s3cret" {
		t.Errorf("password = %q, want %q", data["password"], "s3cret")
	}
}
