package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dvflw/mantle/internal/audit"
)

// TokenValidator validates bearer tokens and returns the claims.
type TokenValidator interface {
	ValidateToken(ctx context.Context, rawToken string) (*OIDCClaims, error)
}

type contextKeyType string

const userContextKey contextKeyType = "auth.user"
const authMethodKey contextKeyType = "auth.method"

// WithUser stores the authenticated user in the context.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext retrieves the authenticated user from the context.
func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userContextKey).(*User)
	return u
}

func withAuthMethod(ctx context.Context, method string) context.Context {
	return context.WithValue(ctx, authMethodKey, method)
}

// AuthMethodFromContext retrieves the authentication method ("api_key" or "oidc") from the context.
func AuthMethodFromContext(ctx context.Context) string {
	m, _ := ctx.Value(authMethodKey).(string)
	return m
}

// AuthMiddleware extracts the credential from the Authorization header,
// sniffs the format (API key vs OIDC JWT), authenticates the user,
// and stores it in the request context.
// Health check endpoints are excluded from authentication.
// The optional auditor parameter, when non-nil, emits auth.failed audit events
// on authentication failures.
func AuthMiddleware(store *Store, oidcValidator TokenValidator, next http.Handler, auditor ...audit.Emitter) http.Handler {
	var emitter audit.Emitter
	if len(auditor) > 0 {
		emitter = auditor[0]
	}

	emitAuthFailure := func(r *http.Request, method, reason string) {
		if emitter == nil {
			return
		}
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.Split(forwarded, ",")[0]
		}
		_ = emitter.Emit(r.Context(), audit.Event{
			Action: audit.ActionAuthFailed,
			Actor:  clientIP,
			Resource: audit.Resource{
				Type: "auth",
				ID:   r.URL.Path,
			},
			Metadata: map[string]string{
				"method":    method,
				"reason":    reason,
				"client_ip": clientIP,
			},
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks only. /metrics requires authentication.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			emitAuthFailure(r, "none", "missing Authorization header")
			http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
			return
		}

		rawToken := strings.TrimPrefix(authHeader, "Bearer ")
		if rawToken == authHeader {
			emitAuthFailure(r, "unknown", "non-Bearer scheme")
			http.Error(w, `{"error":"Authorization header must use Bearer scheme"}`, http.StatusUnauthorized)
			return
		}

		var user *User
		var authMethod string
		var err error

		switch {
		case strings.HasPrefix(rawToken, "mk_"):
			// API key path.
			authMethod = "api_key"
			user, err = store.LookupAPIKey(r.Context(), rawToken)
			if err != nil {
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if user == nil {
				emitAuthFailure(r, "api_key", "invalid API key")
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

		case strings.Count(rawToken, ".") == 2 && oidcValidator != nil:
			// OIDC JWT path (three-part structure: header.payload.signature).
			authMethod = "oidc"
			claims, vErr := oidcValidator.ValidateToken(r.Context(), rawToken)
			if vErr != nil {
				slog.Warn("OIDC token validation failed", "error", vErr.Error())
				emitAuthFailure(r, "oidc", "invalid or expired token")
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			user, err = store.LookupUserByEmail(r.Context(), claims.Email)
			if err != nil {
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if user == nil {
				emitAuthFailure(r, "oidc", "no user account for email")
				http.Error(w, `{"error":"no user account for this email"}`, http.StatusUnauthorized)
				return
			}

		default:
			// Return a generic message regardless of why the token wasn't recognized
			// to avoid leaking whether OIDC is configured.
			emitAuthFailure(r, "unknown", "unrecognized credential format")
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}

		ctx := WithUser(r.Context(), user)
		ctx = withAuthMethod(ctx, authMethod)
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

// TeamIDFromContext extracts the team ID from the authenticated user in the context.
// Returns DefaultTeamID for single-tenant mode (no auth / no user in context).
func TeamIDFromContext(ctx context.Context) string {
	user := UserFromContext(ctx)
	if user == nil || user.TeamID == "" {
		return DefaultTeamID
	}
	return user.TeamID
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
