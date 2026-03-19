package secret

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Resolver resolves credential names to their field values.
// It checks the Postgres store first, then falls back to environment variables.
type Resolver struct {
	Store *Store
}

// Resolve looks up a credential by name and returns its decrypted field data.
// Resolution order:
//  1. Postgres credentials table
//  2. Environment variable MANTLE_SECRET_<UPPER_NAME>
func (r *Resolver) Resolve(ctx context.Context, name string) (map[string]string, error) {
	// Try Postgres store first.
	if r.Store != nil {
		data, err := r.Store.Get(ctx, name)
		if err == nil {
			return data, nil
		}
		// If not found, fall through to env var. Other errors are real.
		if !strings.Contains(err.Error(), "not found") {
			return nil, err
		}
	}

	// Fallback: environment variable.
	envKey := "MANTLE_SECRET_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if val := os.Getenv(envKey); val != "" {
		return map[string]string{"key": val}, nil
	}

	return nil, fmt.Errorf("credential %q not found (checked database and env var %s)", name, envKey)
}
