package sync

import (
	"context"
	"errors"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

func TestNewAuthResolver(t *testing.T) {
	ctx := context.Background()

	t.Run("token credential returns BasicAuth with default username", func(t *testing.T) {
		lookup := func(_ context.Context, name string) (map[string]string, error) {
			return map[string]string{"token": "ghp_secret123"}, nil
		}
		fn := newAuthResolverFromLookup(ctx, lookup)
		auth, err := fn("my-cred")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		basic, ok := auth.(*http.BasicAuth)
		if !ok {
			t.Fatalf("expected *http.BasicAuth, got %T", auth)
		}
		if basic.Password != "ghp_secret123" {
			t.Errorf("password = %q, want %q", basic.Password, "ghp_secret123")
		}
		if basic.Username != "x-token-auth" {
			t.Errorf("username = %q, want %q (default)", basic.Username, "x-token-auth")
		}
	})

	t.Run("token credential with explicit username preserves it", func(t *testing.T) {
		lookup := func(_ context.Context, name string) (map[string]string, error) {
			return map[string]string{"token": "ghp_secret123", "username": "mybot"}, nil
		}
		fn := newAuthResolverFromLookup(ctx, lookup)
		auth, err := fn("my-cred")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		basic, ok := auth.(*http.BasicAuth)
		if !ok {
			t.Fatalf("expected *http.BasicAuth, got %T", auth)
		}
		if basic.Username != "mybot" {
			t.Errorf("username = %q, want %q", basic.Username, "mybot")
		}
	})

	t.Run("ssh_key credential returns unsupported error", func(t *testing.T) {
		lookup := func(_ context.Context, name string) (map[string]string, error) {
			return map[string]string{"ssh_key": "-----BEGIN OPENSSH PRIVATE KEY-----"}, nil
		}
		fn := newAuthResolverFromLookup(ctx, lookup)
		_, err := fn("my-ssh-cred")
		if err == nil {
			t.Fatal("expected error for ssh_key credential, got nil")
		}
		if !containsString(err.Error(), "ssh_key credentials are not supported") {
			t.Errorf("error = %q, want SSH not supported message", err.Error())
		}
	})

	t.Run("credential with no token or ssh_key returns error", func(t *testing.T) {
		lookup := func(_ context.Context, name string) (map[string]string, error) {
			return map[string]string{"username": "someone"}, nil
		}
		fn := newAuthResolverFromLookup(ctx, lookup)
		_, err := fn("empty-cred")
		if err == nil {
			t.Fatal("expected error for empty credential, got nil")
		}
		if !containsString(err.Error(), "no token or ssh_key") {
			t.Errorf("error = %q, want 'no token or ssh_key' message", err.Error())
		}
	})

	t.Run("nil lookup returns resolver-not-configured error", func(t *testing.T) {
		fn := newAuthResolverFromLookup(ctx, nil)
		_, err := fn("any-cred")
		if err == nil {
			t.Fatal("expected error for nil resolver, got nil")
		}
		if !containsString(err.Error(), "secret resolver not configured") {
			t.Errorf("error = %q, want 'secret resolver not configured'", err.Error())
		}
	})

	t.Run("lookup error is wrapped and returned", func(t *testing.T) {
		lookup := func(_ context.Context, name string) (map[string]string, error) {
			return nil, errors.New("credential not found")
		}
		fn := newAuthResolverFromLookup(ctx, lookup)
		_, err := fn("missing-cred")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !containsString(err.Error(), "resolving credential") {
			t.Errorf("error = %q, want wrapping 'resolving credential'", err.Error())
		}
		if !containsString(err.Error(), "credential not found") {
			t.Errorf("error = %q, want wrapped 'credential not found'", err.Error())
		}
	})
}

// containsString is a small helper to avoid importing strings in test assertions.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i+len(substr) <= len(s); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
