// Package cli implements the Mantle command-line interface.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

const (
	CredTypeAPIKey = "api_key"
	CredTypeOIDC   = "oidc"
)

// Credentials holds the authentication material persisted to disk.
type Credentials struct {
	Type         string    `json:"type"`
	APIKey       string    `json:"api_key,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// DefaultCredentialsPath returns ~/.mantle/credentials, the default location
// where authentication material is stored.
func DefaultCredentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mantle", "credentials")
}

// SaveCredentials marshals cred to JSON and writes it to path with
// 0600 permissions, creating parent directories as needed.
func SaveCredentials(path string, cred *Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	return nil
}

// LoadCredentials reads and parses the JSON credentials file at path.
func LoadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}
	var cred Credentials
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	return &cred, nil
}

// DeleteCredentials removes the credentials file at path. It is not an error
// if the file does not exist.
func DeleteCredentials(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing credentials: %w", err)
	}
	return nil
}

// ResolveCredentials returns credentials using the precedence chain:
// 1. --api-key flag  2. MANTLE_API_KEY env  3. MANTLE_OIDC_TOKEN env
// 4. ~/.mantle/credentials (with auto-refresh)  5. Error
func ResolveCredentials(apiKeyFlag, oidcTokenFlag, credPath string, tokenSource oauth2.TokenSource) (*Credentials, error) {
	if apiKeyFlag != "" {
		return &Credentials{Type: CredTypeAPIKey, APIKey: apiKeyFlag}, nil
	}
	if key := os.Getenv("MANTLE_API_KEY"); key != "" {
		return &Credentials{Type: CredTypeAPIKey, APIKey: key}, nil
	}
	if token := os.Getenv("MANTLE_OIDC_TOKEN"); token != "" {
		return &Credentials{Type: CredTypeOIDC, AccessToken: token}, nil
	}
	cred, err := LoadCredentials(credPath)
	if err != nil {
		return nil, fmt.Errorf("no credentials found — run 'mantle login' to authenticate")
	}
	// Auto-refresh expired OIDC tokens
	if cred.Type == CredTypeOIDC && !cred.ExpiresAt.IsZero() && time.Now().After(cred.ExpiresAt) {
		if tokenSource == nil || cred.RefreshToken == "" {
			return nil, fmt.Errorf("OIDC token expired — run 'mantle login' to re-authenticate")
		}
		newToken, err := tokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("token refresh failed — run 'mantle login' to re-authenticate: %w", err)
		}
		cred.AccessToken = newToken.AccessToken
		cred.RefreshToken = newToken.RefreshToken
		cred.ExpiresAt = newToken.Expiry
		if saveErr := SaveCredentials(credPath, cred); saveErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save refreshed token: %v\n", saveErr)
		}
	}
	return cred, nil
}
