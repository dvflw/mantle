package secret

import (
	"context"
	"encoding/json"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	gax "github.com/googleapis/gax-go/v2"
)

// smClient is the subset of the Secret Manager API used by this backend.
// It exists so we can inject a mock in tests.
type smClient interface {
	AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error)
}

// GCPSecretManagerBackend resolves credentials from GCP Secret Manager.
// It uses Application Default Credentials (ADC) for authentication.
type GCPSecretManagerBackend struct {
	projectID string
	client    smClient
}

// NewGCPSecretManagerBackend creates a backend that fetches secrets from the
// given GCP project. It initialises a Secret Manager client using ADC.
func NewGCPSecretManagerBackend(ctx context.Context, projectID string) (*GCPSecretManagerBackend, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating secret manager client: %w", err)
	}
	return &GCPSecretManagerBackend{
		projectID: projectID,
		client:    client,
	}, nil
}

// newGCPSecretManagerBackendWithClient is used in tests to inject a mock client.
func newGCPSecretManagerBackendWithClient(projectID string, client smClient) *GCPSecretManagerBackend {
	return &GCPSecretManagerBackend{
		projectID: projectID,
		client:    client,
	}
}

// Resolve fetches the latest version of the named secret from GCP Secret Manager.
// If the payload is valid JSON that decodes to map[string]string, it is returned
// as-is. Otherwise the raw string value is returned under the key "key".
func (b *GCPSecretManagerBackend) Resolve(ctx context.Context, name string) (map[string]string, error) {
	secretName := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", b.projectID, name)

	resp, err := b.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: secretName,
	})
	if err != nil {
		return nil, fmt.Errorf("accessing secret %q: %w", name, err)
	}

	payload := resp.GetPayload()
	if payload == nil {
		return nil, fmt.Errorf("secret %q has no payload", name)
	}

	data := payload.GetData()
	if len(data) == 0 {
		return nil, fmt.Errorf("secret %q payload is empty", name)
	}

	// Try to parse as JSON map.
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err == nil {
		return fields, nil
	}

	// Fallback: treat as an opaque string value.
	return map[string]string{"key": string(data)}, nil
}
