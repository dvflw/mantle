package secret

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// mockAzSecretsClient implements azSecretsClient for testing.
type mockAzSecretsClient struct {
	value *string
	err   error
}

func (m *mockAzSecretsClient) GetSecret(_ context.Context, _ string, _ string, _ *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error) {
	if m.err != nil {
		return azsecrets.GetSecretResponse{}, m.err
	}
	resp := azsecrets.GetSecretResponse{}
	resp.Value = m.value
	return resp, nil
}

func strPtr(s string) *string { return &s }

func TestAzureKeyVaultBackend_ResolveJSON(t *testing.T) {
	mock := &mockAzSecretsClient{
		value: strPtr(`{"username":"admin","password":"s3cret"}`),
	}
	backend := newAzureKeyVaultBackendWithClient(mock)

	data, err := backend.Resolve(context.Background(), "my-cred")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data["username"] != "admin" {
		t.Errorf("username = %q, want %q", data["username"], "admin")
	}
	if data["password"] != "s3cret" {
		t.Errorf("password = %q, want %q", data["password"], "s3cret")
	}
	if len(data) != 2 {
		t.Errorf("got %d fields, want 2", len(data))
	}
}

func TestAzureKeyVaultBackend_ResolveNonJSON(t *testing.T) {
	mock := &mockAzSecretsClient{
		value: strPtr("plain-api-key-value"),
	}
	backend := newAzureKeyVaultBackendWithClient(mock)

	data, err := backend.Resolve(context.Background(), "my-api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data["key"] != "plain-api-key-value" {
		t.Errorf("key = %q, want %q", data["key"], "plain-api-key-value")
	}
	if len(data) != 1 {
		t.Errorf("got %d fields, want 1", len(data))
	}
}

func TestAzureKeyVaultBackend_ResolveNilValue(t *testing.T) {
	mock := &mockAzSecretsClient{
		value: nil,
	}
	backend := newAzureKeyVaultBackendWithClient(mock)

	_, err := backend.Resolve(context.Background(), "empty-secret")
	if err == nil {
		t.Fatal("expected error for nil value, got nil")
	}
}

func TestAzureKeyVaultBackend_ResolveError(t *testing.T) {
	mock := &mockAzSecretsClient{
		err: fmt.Errorf("vault unavailable"),
	}
	backend := newAzureKeyVaultBackendWithClient(mock)

	_, err := backend.Resolve(context.Background(), "unreachable")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
