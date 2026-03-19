package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKeyType string

const userContextKey contextKeyType = "auth.user"

// WithUser stores the authenticated user in the context.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext retrieves the authenticated user from the context.
func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userContextKey).(*User)
	return u
}

// AuthMiddleware extracts the API key from the Authorization header,
// looks up the user, and stores it in the request context.
// Health check endpoints are excluded from authentication.
func AuthMiddleware(store *Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
			return
		}

		rawKey := strings.TrimPrefix(authHeader, "Bearer ")
		if rawKey == authHeader {
			http.Error(w, `{"error":"Authorization header must use Bearer scheme"}`, http.StatusUnauthorized)
			return
		}

		user, err := store.LookupAPIKey(r.Context(), rawKey)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
			return
		}

		ctx := WithUser(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole returns middleware that checks the authenticated user has
// at least the required role level.
// Role hierarchy: admin > team_owner > operator
func RequireRole(minRole Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if !hasMinRole(user.Role, minRole) {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// hasMinRole checks if userRole meets the minimum required role.
func hasMinRole(userRole, minRole Role) bool {
	levels := map[Role]int{
		RoleOperator:  1,
		RoleTeamOwner: 2,
		RoleAdmin:     3,
	}

	userLevel, ok1 := levels[userRole]
	minLevel, ok2 := levels[minRole]
	if !ok1 || !ok2 {
		return false
	}
	return userLevel >= minLevel
}
