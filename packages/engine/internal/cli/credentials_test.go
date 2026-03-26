package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCredentials_APIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	cred := &Credentials{
		Type:   CredTypeAPIKey,
		APIKey: "mk_test_abc123",
	}

	if err := SaveCredentials(path, cred); err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	// Verify file permissions are 0600.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected permissions 0600, got %04o", perm)
	}

	loaded, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}
	if loaded.Type != CredTypeAPIKey {
		t.Errorf("expected type %q, got %q", CredTypeAPIKey, loaded.Type)
	}
	if loaded.APIKey != "mk_test_abc123" {
		t.Errorf("expected api_key %q, got %q", "mk_test_abc123", loaded.APIKey)
	}
}

func TestSaveAndLoadCredentials_OIDC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	expiry := time.Now().Add(time.Hour).Truncate(time.Second)
	cred := &Credentials{
		Type:         CredTypeOIDC,
		AccessToken:  "eyJhbGciOi.access",
		RefreshToken: "eyJhbGciOi.refresh",
		ExpiresAt:    expiry,
	}

	if err := SaveCredentials(path, cred); err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	loaded, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}
	if loaded.Type != CredTypeOIDC {
		t.Errorf("expected type %q, got %q", CredTypeOIDC, loaded.Type)
	}
	if loaded.AccessToken != "eyJhbGciOi.access" {
		t.Errorf("expected access_token %q, got %q", "eyJhbGciOi.access", loaded.AccessToken)
	}
	if loaded.RefreshToken != "eyJhbGciOi.refresh" {
		t.Errorf("expected refresh_token %q, got %q", "eyJhbGciOi.refresh", loaded.RefreshToken)
	}
	if !loaded.ExpiresAt.Equal(expiry) {
		t.Errorf("expected expires_at %v, got %v", expiry, loaded.ExpiresAt)
	}
}

func TestLoadCredentials_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent")
	_, err := LoadCredentials(path)
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}

func TestDeleteCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	cred := &Credentials{Type: CredTypeAPIKey, APIKey: "to-delete"}
	if err := SaveCredentials(path, cred); err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	if err := DeleteCredentials(path); err != nil {
		t.Fatalf("DeleteCredentials failed: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, but stat returned: %v", err)
	}

	// Deleting again should not error (idempotent).
	if err := DeleteCredentials(path); err != nil {
		t.Fatalf("DeleteCredentials on missing file failed: %v", err)
	}
}

func TestResolveCredentials_Precedence(t *testing.T) {
	// Set up a cached credentials file.
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	cred := &Credentials{Type: CredTypeAPIKey, APIKey: "cached-key"}
	if err := SaveCredentials(path, cred); err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	// Set env var — should win over cached file.
	t.Setenv("MANTLE_API_KEY", "env-key")
	t.Setenv("MANTLE_OIDC_TOKEN", "")

	resolved, err := ResolveCredentials("", "", path, nil)
	if err != nil {
		t.Fatalf("ResolveCredentials failed: %v", err)
	}
	if resolved.APIKey != "env-key" {
		t.Errorf("expected env var key %q, got %q", "env-key", resolved.APIKey)
	}

	// Flag should win over env var.
	resolved, err = ResolveCredentials("flag-key", "", path, nil)
	if err != nil {
		t.Fatalf("ResolveCredentials failed: %v", err)
	}
	if resolved.APIKey != "flag-key" {
		t.Errorf("expected flag key %q, got %q", "flag-key", resolved.APIKey)
	}

	// With no flag and no env, cached file should be used.
	t.Setenv("MANTLE_API_KEY", "")
	resolved, err = ResolveCredentials("", "", path, nil)
	if err != nil {
		t.Fatalf("ResolveCredentials failed: %v", err)
	}
	if resolved.APIKey != "cached-key" {
		t.Errorf("expected cached key %q, got %q", "cached-key", resolved.APIKey)
	}
}

func TestResolveCredentials_OIDCEnvVar(t *testing.T) {
	t.Setenv("MANTLE_API_KEY", "")
	t.Setenv("MANTLE_OIDC_TOKEN", "oidc-from-env")

	// credPath points to nonexistent file — env var should still resolve.
	path := filepath.Join(t.TempDir(), "nonexistent")

	resolved, err := ResolveCredentials("", "", path, nil)
	if err != nil {
		t.Fatalf("ResolveCredentials failed: %v", err)
	}
	if resolved.Type != CredTypeOIDC {
		t.Errorf("expected type %q, got %q", CredTypeOIDC, resolved.Type)
	}
	if resolved.AccessToken != "oidc-from-env" {
		t.Errorf("expected access_token %q, got %q", "oidc-from-env", resolved.AccessToken)
	}
}
