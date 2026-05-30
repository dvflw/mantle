package sync

import (
	"context"
	"fmt"

	"github.com/dvflw/mantle/internal/secret"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// authLookup is a function that resolves a credential name to its field map.
// Defined as a type so tests can inject a fake without needing a live DB.
type authLookup func(ctx context.Context, name string) (map[string]string, error)

// NewAuthResolver returns a callback suitable for GoGitDriver.Auth. The
// resolver looks up a credential by name via the provided resolver, then
// converts the git-type fields into a go-git AuthMethod.
//
// Only HTTPS tokens are supported in this release. SSH keys return an
// explicit error so operators see a clear message instead of a silent
// authentication failure — SSH support lands in Plan C.
func NewAuthResolver(ctx context.Context, resolver *secret.Resolver) func(string) (transport.AuthMethod, error) {
	if resolver == nil {
		return newAuthResolverFromLookup(ctx, nil)
	}
	return newAuthResolverFromLookup(ctx, resolver.Resolve)
}

// newAuthResolverFromLookup builds the Auth callback from any authLookup
// function. It is the testable core; NewAuthResolver adapts the production
// secret.Resolver to this interface.
func newAuthResolverFromLookup(ctx context.Context, lookup authLookup) func(string) (transport.AuthMethod, error) {
	return func(name string) (transport.AuthMethod, error) {
		if lookup == nil {
			return nil, fmt.Errorf("secret resolver not configured")
		}
		fields, err := lookup(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("resolving credential %q: %w", name, err)
		}
		if token := fields["token"]; token != "" {
			username := fields["username"]
			if username == "" {
				username = "x-token-auth"
			}
			return &http.BasicAuth{Username: username, Password: token}, nil
		}
		if fields["ssh_key"] != "" {
			return nil, fmt.Errorf("ssh_key credentials are not supported yet; use a token credential for now (SSH auth ships in Plan C)")
		}
		return nil, fmt.Errorf("credential %q has no token or ssh_key", name)
	}
}
