package secret

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// SecretBackend is the interface for external secret resolution backends.
// Implementations return the secret's field data, or an error if the secret
// cannot be found or accessed.
type SecretBackend interface {
	Resolve(ctx context.Context, name string) (map[string]string, error)
}

// Resolver resolves credential names to their field values.
// Resolution order:
//  1. Postgres store (if configured)
//  2. External backends in registration order
//  3. Environment variable fallback
type Resolver struct {
	Store    *Store
	Backends []SecretBackend
}

// Resolve looks up a credential by name and returns its decrypted field data.
func (r *Resolver) Resolve(ctx context.Context, name string) (map[string]string, error) {
	// 1. Try Postgres store first.
	if r.Store != nil {
		data, err := r.Store.Get(ctx, name)
		if err == nil {
			return data, nil
		}
		// If not found, fall through. Other errors are real.
		if !strings.Contains(err.Error(), "not found") {
			return nil, err
		}
	}

	// 2. Try each external backend in order.
	for _, b := range r.Backends {
		data, err := b.Resolve(ctx, name)
		if err == nil {
			return data, nil
		}
		// Backends return errors for both "not found" and real failures.
		// We fall through on any error so the next backend (or env var
		// fallback) gets a chance. If all sources fail the final error
		// message covers it.
	}

	// 3. Fallback: environment variable.
	envKey := "MANTLE_SECRET_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if val := os.Getenv(envKey); val != "" {
		return map[string]string{"key": val}, nil
	}

	return nil, fmt.Errorf("credential %q not found (checked database, %d backend(s), and env var %s)", name, len(r.Backends), envKey)
}
