package secret

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// azSecretsClient is the subset of the Azure Key Vault secrets client needed
// by this backend. It exists so callers can supply a mock in tests.
type azSecretsClient interface {
	GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

// AzureKeyVaultBackend resolves credentials from Azure Key Vault.
// The secret name in Azure maps 1:1 to the credential name in Mantle.
type AzureKeyVaultBackend struct {
	client azSecretsClient
}

// NewAzureKeyVaultBackend creates a backend that fetches secrets from the
// given Azure Key Vault URL (e.g., "https://myvault.vault.azure.net/").
// It uses Azure Default Credential (env vars, managed identity, Azure CLI, etc.).
func NewAzureKeyVaultBackend(vaultURL string) (*AzureKeyVaultBackend, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("creating Azure credential: %w", err)
	}

	client, err := azsecrets.NewClient(vaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Azure Key Vault client: %w", err)
	}

	return &AzureKeyVaultBackend{client: client}, nil
}

// newAzureKeyVaultBackendWithClient creates a backend from a pre-built client.
// This is the primary constructor for tests.
func newAzureKeyVaultBackendWithClient(client azSecretsClient) *AzureKeyVaultBackend {
	return &AzureKeyVaultBackend{client: client}
}

// Resolve fetches a secret by name from Azure Key Vault.
// An empty version string fetches the latest version.
// If the secret value is valid JSON, it is decoded into map[string]string.
// Otherwise the raw string is returned as {"key": "<value>"}.
func (b *AzureKeyVaultBackend) Resolve(ctx context.Context, name string) (map[string]string, error) {
	resp, err := b.client.GetSecret(ctx, name, "", nil)
	if err != nil {
		return nil, fmt.Errorf("fetching secret %q from Azure Key Vault: %w", name, err)
	}

	if resp.Value == nil {
		return nil, fmt.Errorf("secret %q has no value", name)
	}

	raw := *resp.Value

	// Attempt JSON parse first.
	var data map[string]string
	if err := json.Unmarshal([]byte(raw), &data); err == nil {
		return data, nil
	}

	// Fallback: treat the entire value as a single key.
	return map[string]string{"key": raw}, nil
}
