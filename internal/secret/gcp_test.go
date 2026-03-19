package secret

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	gax "github.com/googleapis/gax-go/v2"
)

// mockGCPSMClient implements smClient for testing.
type mockGCPSMClient struct {
	response *secretmanagerpb.AccessSecretVersionResponse
	err      error
	// captured holds the last request for assertion.
	captured *secretmanagerpb.AccessSecretVersionRequest
}

func (m *mockGCPSMClient) AccessSecretVersion(_ context.Context, req *secretmanagerpb.AccessSecretVersionRequest, _ ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	m.captured = req
	return m.response, m.err
}

func TestGCPSecretManagerBackend_ResolveJSON(t *testing.T) {
	mock := &mockGCPSMClient{
		response: &secretmanagerpb.AccessSecretVersionResponse{
			Payload: &secretmanagerpb.SecretPayload{
				Data: []byte(`{"username":"admin","password":"s3cret"}`),
			},
		},
	}

	backend := newGCPSecretManagerBackendWithClient("my-project", mock)

	data, err := backend.Resolve(context.Background(), "db-creds")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data["username"] != "admin" {
		t.Errorf("username = %q, want %q", data["username"], "admin")
	}
	if data["password"] != "s3cret" {
		t.Errorf("password = %q, want %q", data["password"], "s3cret")
	}

	// Verify the correct secret name was requested.
	wantName := "projects/my-project/secrets/db-creds/versions/latest"
	if mock.captured.Name != wantName {
		t.Errorf("requested name = %q, want %q", mock.captured.Name, wantName)
	}
}

func TestGCPSecretManagerBackend_ResolveNonJSON(t *testing.T) {
	mock := &mockGCPSMClient{
		response: &secretmanagerpb.AccessSecretVersionResponse{
			Payload: &secretmanagerpb.SecretPayload{
				Data: []byte("my-api-key-value"),
			},
		},
	}

	backend := newGCPSecretManagerBackendWithClient("my-project", mock)

	data, err := backend.Resolve(context.Background(), "api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data) != 1 {
		t.Fatalf("expected 1 field, got %d", len(data))
	}
	if data["key"] != "my-api-key-value" {
		t.Errorf("key = %q, want %q", data["key"], "my-api-key-value")
	}
}

func TestGCPSecretManagerBackend_ResolveError(t *testing.T) {
	mock := &mockGCPSMClient{
		err: fmt.Errorf("rpc error: code = NotFound desc = Secret not found"),
	}

	backend := newGCPSecretManagerBackendWithClient("my-project", mock)

	_, err := backend.Resolve(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `accessing secret "missing": rpc error: code = NotFound desc = Secret not found` {
		t.Errorf("error = %q, want wrapped NotFound", got)
	}
}

func TestGCPSecretManagerBackend_NilPayload(t *testing.T) {
	mock := &mockGCPSMClient{
		response: &secretmanagerpb.AccessSecretVersionResponse{
			Payload: nil,
		},
	}

	backend := newGCPSecretManagerBackendWithClient("my-project", mock)

	_, err := backend.Resolve(context.Background(), "empty-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `secret "empty-secret" has no payload` {
		t.Errorf("error = %q", got)
	}
}

func TestGCPSecretManagerBackend_EmptyPayload(t *testing.T) {
	mock := &mockGCPSMClient{
		response: &secretmanagerpb.AccessSecretVersionResponse{
			Payload: &secretmanagerpb.SecretPayload{
				Data: []byte{},
			},
		},
	}

	backend := newGCPSecretManagerBackendWithClient("my-project", mock)

	_, err := backend.Resolve(context.Background(), "empty-data")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `secret "empty-data" payload is empty` {
		t.Errorf("error = %q", got)
	}
}

func TestGCPSecretManagerBackend_ImplementsInterface(t *testing.T) {
	// Compile-time check that GCPSecretManagerBackend satisfies SecretBackend.
	var _ SecretBackend = (*GCPSecretManagerBackend)(nil)
}
