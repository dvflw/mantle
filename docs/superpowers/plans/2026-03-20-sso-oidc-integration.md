# SSO/OIDC Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OIDC-based SSO authentication alongside existing API key auth, with `mantle login` CLI for token management.

**Architecture:** Server-side OIDC JWT validation via `coreos/go-oidc` with token-format sniffing in existing `AuthMiddleware` (`mk_` prefix → API key, JWT with dots → OIDC). CLI gets `mantle login` (auth code + PKCE default, device flow via `--device`) and `mantle logout` commands. Credentials cached at `~/.mantle/credentials` (0600). No JIT user provisioning — users must be pre-created via `mantle users create`.

**Tech Stack:** `github.com/coreos/go-oidc/v3` (OIDC discovery + JWKS), `golang.org/x/oauth2` (already in go.mod — OAuth2 flows), Go standard library (`crypto`, `net/http`)

**Linear Issue:** DVFLW-265

---

## File Structure

| File | Responsibility |
|------|---------------|
| **Create:** `internal/auth/oidc.go` | `OIDCValidator` struct — JWKS-cached JWT validation, email extraction, domain enforcement |
| **Create:** `internal/auth/oidc_test.go` | Unit tests with mock OIDC server + integration test for full JWT→user chain |
| **Create:** `internal/auth/test_helpers_test.go` | Shared test helpers: `mockOIDCServer`, `signTestJWT` (used by oidc_test.go and auth_test.go) |
| **Modify:** `internal/auth/middleware.go` | Token-format sniffing in `AuthMiddleware` — route to API key or OIDC path |
| **Modify:** `internal/auth/models.go` | Add `AuthMethod` type constant for audit context |
| **Modify:** `internal/config/config.go` | Add `AuthConfig` with `OIDCConfig` nested struct (including `ClientSecret`) |
| **Modify:** `internal/cli/serve.go` | Wire `OIDCValidator` into server when OIDC config present |
| **Create:** `internal/cli/login.go` | `mantle login`, `mantle login --device`, `mantle login --api-key`, `mantle logout` |
| **Create:** `internal/cli/browser_darwin.go` | Platform-specific browser open (macOS) |
| **Create:** `internal/cli/browser_linux.go` | Platform-specific browser open (Linux) |
| **Create:** `internal/cli/browser_windows.go` | Platform-specific browser open (Windows) |
| **Create:** `internal/cli/credentials.go` | Credential file read/write/resolve (`~/.mantle/credentials`, 0600) |
| **Create:** `internal/cli/credentials_test.go` | Tests for credential resolution chain, token refresh, file permissions |
| **Modify:** `internal/cli/root.go` | Register `login`/`logout` commands, add `--api-key` persistent flag |
| **Modify:** `internal/server/server.go:147` | Accept `*auth.OIDCValidator` on `Server` struct, pass to middleware |

**Important:** Tasks 1→2→3→4 are strictly sequential (each depends on the previous). Tasks 5→6→7 are a separate sequential chain. The two chains can run in parallel. Task 8 depends on both chains.

---

### Task 1: Add OIDC config to config system

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_oidc_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOIDCConfig(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(cfgFile, []byte(`
auth:
  oidc:
    issuer_url: "https://accounts.google.com"
    client_id: "test-client-id"
    audience: "mantle"
    allowed_domains:
      - "example.com"
      - "corp.example.com"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Load config directly — match existing config test pattern
	cmd := newTestCommand()
	cmd.SetArgs([]string{"--config", cfgFile})
	_ = cmd.Execute()

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.OIDC.IssuerURL != "https://accounts.google.com" {
		t.Errorf("issuer_url = %q, want %q", cfg.Auth.OIDC.IssuerURL, "https://accounts.google.com")
	}
	if cfg.Auth.OIDC.ClientID != "test-client-id" {
		t.Errorf("client_id = %q, want %q", cfg.Auth.OIDC.ClientID, "test-client-id")
	}
	if cfg.Auth.OIDC.Audience != "mantle" {
		t.Errorf("audience = %q, want %q", cfg.Auth.OIDC.Audience, "mantle")
	}
	if len(cfg.Auth.OIDC.AllowedDomains) != 2 {
		t.Errorf("allowed_domains len = %d, want 2", len(cfg.Auth.OIDC.AllowedDomains))
	}
}

func TestLoadOIDCConfig_Empty(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "mantle.yaml")
	err := os.WriteFile(cfgFile, []byte(`
database:
  url: "postgres://localhost/mantle"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newTestCommand()
	cmd.SetArgs([]string{"--config", cfgFile})
	_ = cmd.Execute()

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.OIDC.IssuerURL != "" {
		t.Errorf("issuer_url should be empty when not configured, got %q", cfg.Auth.OIDC.IssuerURL)
	}
}
```

Note: `newTestCommand()` may need to be a small helper that calls `Load()` and stores via `WithContext`. Check how existing config tests work and adapt.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadOIDCConfig -v`
Expected: FAIL — `cfg.Auth` field does not exist

- [ ] **Step 3: Add the config structs and env var bindings**

In `internal/config/config.go`, add to the `Config` struct:

```go
type Config struct {
	Database   DatabaseConfig   `mapstructure:"database"`
	API        APIConfig        `mapstructure:"api"`
	Log        LogConfig        `mapstructure:"log"`
	Encryption EncryptionConfig `mapstructure:"encryption"`
	Engine     EngineConfig     `mapstructure:"engine"`
	Auth       AuthConfig       `mapstructure:"auth"`
}

type AuthConfig struct {
	OIDC OIDCConfig `mapstructure:"oidc"`
}

type OIDCConfig struct {
	IssuerURL      string   `mapstructure:"issuer_url"`
	ClientID       string   `mapstructure:"client_id"`
	ClientSecret   string   `mapstructure:"client_secret"` // Required for token exchange in mantle login
	Audience       string   `mapstructure:"audience"`
	AllowedDomains []string `mapstructure:"allowed_domains"`
}
```

Add env var bindings in `Load()`:

```go
_ = v.BindEnv("auth.oidc.issuer_url", "MANTLE_AUTH_OIDC_ISSUER_URL")
_ = v.BindEnv("auth.oidc.client_id", "MANTLE_AUTH_OIDC_CLIENT_ID")
_ = v.BindEnv("auth.oidc.client_secret", "MANTLE_AUTH_OIDC_CLIENT_SECRET")
_ = v.BindEnv("auth.oidc.audience", "MANTLE_AUTH_OIDC_AUDIENCE")
// Note: allowed_domains is best configured via YAML (array type).
// Viper supports comma-separated env vars for slices:
// MANTLE_AUTH_OIDC_ALLOWED_DOMAINS="example.com,corp.example.com"
_ = v.BindEnv("auth.oidc.allowed_domains", "MANTLE_AUTH_OIDC_ALLOWED_DOMAINS")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoadOIDCConfig -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_oidc_test.go
git commit -m "feat(auth): add OIDC config struct and env var bindings"
```

---

### Task 2: OIDC JWT validator

**Files:**
- Create: `internal/auth/oidc.go`
- Create: `internal/auth/oidc_test.go`

- [ ] **Step 1: Create shared test helpers**

Create `internal/auth/test_helpers_test.go` with the mock OIDC server and JWT signing helpers. These are shared by `oidc_test.go` (this task) and `auth_test.go` (Task 3 and 8).

```go
package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// mockOIDCServer creates a test HTTP server that serves OIDC discovery and JWKS.
func mockOIDCServer(t *testing.T, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]interface{}{
				{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test-key-1", "n": n, "e": e},
			},
		})
	})

	srv := httptest.NewServer(mux)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issuer":                 srv.URL,
			"jwks_uri":              srv.URL + "/jwks",
			"authorization_endpoint": srv.URL + "/authorize",
			"token_endpoint":         srv.URL + "/token",
			"device_authorization_endpoint": srv.URL + "/device/code",
		})
	})

	t.Cleanup(srv.Close)
	return srv
}

func signTestJWT(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-1"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("signing JWT: %v", err)
	}
	return signed
}
```

- [ ] **Step 2: Write the failing unit tests**

Create `internal/auth/oidc_test.go`. The mock OIDC server serves `/.well-known/openid-configuration` and `/jwks` endpoints, signs JWTs with a test RSA key.

```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Note: mockOIDCServer and signTestJWT are in test_helpers_test.go (shared)

func TestOIDCValidator_ValidToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := mockOIDCServer(t, key)

	v, err := NewOIDCValidator(context.Background(), srv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "alice@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	claims, err := v.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", claims.Email, "alice@example.com")
	}
	if claims.Subject != "user-123" {
		t.Errorf("subject = %q, want %q", claims.Subject, "user-123")
	}
}

func TestOIDCValidator_UnverifiedEmail(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := mockOIDCServer(t, key)

	v, err := NewOIDCValidator(context.Background(), srv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "alice@example.com",
		"email_verified": false,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	_, err = v.ValidateToken(context.Background(), token)
	if err == nil {
		t.Fatal("expected error for unverified email")
	}
}

func TestOIDCValidator_DisallowedDomain(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := mockOIDCServer(t, key)

	v, err := NewOIDCValidator(context.Background(), srv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "alice@evil.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	_, err = v.ValidateToken(context.Background(), token)
	if err == nil {
		t.Fatal("expected error for disallowed domain")
	}
}

func TestOIDCValidator_ExpiredToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := mockOIDCServer(t, key)

	v, err := NewOIDCValidator(context.Background(), srv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "alice@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(-time.Hour).Unix(),
		"iat":            time.Now().Add(-2 * time.Hour).Unix(),
	})

	_, err = v.ValidateToken(context.Background(), token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestOIDCValidator_NoDomainRestriction(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := mockOIDCServer(t, key)

	// Empty allowed_domains = no restriction
	v, err := NewOIDCValidator(context.Background(), srv.URL, "test-client", "mantle", nil)
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "anyone@anywhere.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	claims, err := v.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Email != "anyone@anywhere.com" {
		t.Errorf("email = %q, want %q", claims.Email, "anyone@anywhere.com")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestOIDCValidator -v`
Expected: FAIL — `NewOIDCValidator` and `OIDCClaims` don't exist

- [ ] **Step 4: Implement the OIDC validator**

Create `internal/auth/oidc.go`:

```go
package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCClaims holds the validated claims extracted from an OIDC JWT.
type OIDCClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
}

// OIDCValidator validates OIDC JWTs using provider discovery and JWKS.
type OIDCValidator struct {
	verifier       *oidc.IDTokenVerifier
	allowedDomains []string
}

// NewOIDCValidator creates a validator using OIDC discovery.
// It fetches .well-known/openid-configuration from the issuer.
// JWKS is cached internally by the go-oidc library with automatic refresh.
func NewOIDCValidator(ctx context.Context, issuerURL, clientID, audience string, allowedDomains []string) (*OIDCValidator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery for %s: %w", issuerURL, err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: audience, // audience check — the "aud" claim must match
	})

	return &OIDCValidator{
		verifier:       verifier,
		allowedDomains: allowedDomains,
	}, nil
}

// ValidateToken verifies the JWT signature, expiry, issuer, and audience,
// then extracts and validates the email claim.
func (v *OIDCValidator) ValidateToken(ctx context.Context, rawToken string) (*OIDCClaims, error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("extracting claims: %w", err)
	}

	if claims.Email == "" {
		return nil, fmt.Errorf("token missing email claim")
	}
	if !claims.EmailVerified {
		return nil, fmt.Errorf("email %q is not verified", claims.Email)
	}

	if err := v.checkDomain(claims.Email); err != nil {
		return nil, err
	}

	return &OIDCClaims{
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
	}, nil
}

// checkDomain enforces allowed_domains if configured.
func (v *OIDCValidator) checkDomain(email string) error {
	if len(v.allowedDomains) == 0 {
		return nil
	}

	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid email format: %q", email)
	}
	domain := strings.ToLower(parts[1])

	for _, allowed := range v.allowedDomains {
		if strings.ToLower(allowed) == domain {
			return nil
		}
	}
	return fmt.Errorf("email domain %q not in allowed domains", domain)
}
```

- [ ] **Step 5: Add the go-oidc dependency**

Run: `go get github.com/coreos/go-oidc/v3@latest && go mod tidy`

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run TestOIDCValidator -v`
Expected: All 5 tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/auth/oidc.go internal/auth/oidc_test.go internal/auth/test_helpers_test.go go.mod go.sum
git commit -m "feat(auth): add OIDC JWT validator with JWKS caching and domain enforcement"
```

---

### Task 3: Token-sniffing middleware

**Files:**
- Modify: `internal/auth/middleware.go`
- Modify: `internal/auth/models.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/auth/auth_test.go`:

```go
func TestAuthMiddleware_OIDCToken(t *testing.T) {
	// Set up mock OIDC server and validator
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := mockOIDCServer(t, key)

	store := setupTestStore(t)
	ctx := context.Background()

	// Pre-provision user
	_, err := store.CreateUser(ctx, "alice@example.com", "Alice", DefaultTeamID, RoleOperator)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	validator, err := NewOIDCValidator(ctx, srv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	var capturedUser *User
	handler := AuthMiddleware(store, validator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		w.WriteHeader(200)
	}))

	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "alice@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if capturedUser == nil {
		t.Fatal("user not set in context")
	}
	if capturedUser.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", capturedUser.Email, "alice@example.com")
	}
}

func TestAuthMiddleware_OIDCToken_UserNotFound(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := mockOIDCServer(t, key)

	store := setupTestStore(t)
	ctx := context.Background()

	// NO pre-provisioned user

	validator, err := NewOIDCValidator(ctx, srv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	handler := AuthMiddleware(store, validator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "nobody@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_APIKeyStillWorks(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	user, _ := store.CreateUser(ctx, "bob@example.com", "Bob", DefaultTeamID, RoleAdmin)
	rawKey, _, _ := store.CreateAPIKey(ctx, user.ID, "test-key")

	// Pass nil validator — OIDC not configured
	handler := AuthMiddleware(store, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
```

Note: `mockOIDCServer` and `signTestJWT` helpers are already in `test_helpers_test.go` (created in Task 2, Step 1). Both test files in the `auth` package can use them.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth/ -run "TestAuthMiddleware_OIDC|TestAuthMiddleware_APIKeyStillWorks" -v`
Expected: FAIL — `AuthMiddleware` signature doesn't accept validator parameter

- [ ] **Step 3: Update AuthMiddleware to sniff token format**

Modify `internal/auth/middleware.go`:

```go
// AuthMiddleware extracts credentials from the Authorization header,
// determines the auth method (API key vs OIDC JWT), authenticates,
// and stores the user in the request context.
// The oidcValidator parameter is optional — pass nil to disable OIDC.
func AuthMiddleware(store *Store, oidcValidator *OIDCValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks and metrics.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, `{"error":"Authorization header must use Bearer scheme"}`, http.StatusUnauthorized)
			return
		}

		var user *User
		var authMethod string
		var err error

		if strings.HasPrefix(token, "mk_") {
			// API key path
			authMethod = "api_key"
			user, err = store.LookupAPIKey(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if user == nil {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}
		} else if oidcValidator != nil && strings.Contains(token, ".") {
			// OIDC JWT path — JWTs contain dots separating header.payload.signature
			authMethod = "oidc"
			claims, vErr := oidcValidator.ValidateToken(r.Context(), token)
			if vErr != nil {
				// Log the detailed error server-side, return generic message to client
				slog.Warn("OIDC token validation failed", "error", vErr)
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			user, err = store.LookupUserByEmail(r.Context(), claims.Email)
			if err != nil {
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if user == nil {
				http.Error(w, `{"error":"no Mantle user for email, contact your admin"}`, http.StatusUnauthorized)
				return
			}
		} else {
			http.Error(w, `{"error":"unrecognized credential format"}`, http.StatusUnauthorized)
			return
		}

		ctx := WithUser(r.Context(), user)
		ctx = withAuthMethod(ctx, authMethod)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

Add to `internal/auth/middleware.go`:

```go
const authMethodKey contextKeyType = "auth.method"

func withAuthMethod(ctx context.Context, method string) context.Context {
	return context.WithValue(ctx, authMethodKey, method)
}

// AuthMethodFromContext returns the authentication method used ("api_key" or "oidc").
func AuthMethodFromContext(ctx context.Context) string {
	m, _ := ctx.Value(authMethodKey).(string)
	return m
}
```

- [ ] **Step 4: Add LookupUserByEmail to the store**

Add to `internal/auth/store.go`:

```go
// LookupUserByEmail finds a user by email address.
// Returns nil, nil if no user found (not an error).
func (s *Store) LookupUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, email, name, team_id, role, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.TeamID, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 5: Update all callers of AuthMiddleware**

The signature changed from `AuthMiddleware(store, next)` to `AuthMiddleware(store, oidcValidator, next)`. Update:

- `internal/server/server.go` — pass `s.OIDCValidator` (new field on Server, nil when OIDC not configured)
- `internal/auth/auth_test.go` — update existing tests: pass `nil` as oidcValidator where OIDC isn't under test

In `internal/server/server.go`, add field:

```go
type Server struct {
	// ... existing fields ...
	OIDCValidator *auth.OIDCValidator
}
```

Update middleware wiring:

```go
if s.AuthStore != nil {
	handler = auth.AuthMiddleware(s.AuthStore, s.OIDCValidator, mux)
}
```

- [ ] **Step 6: Run all auth tests**

Run: `go test ./internal/auth/ -v`
Expected: All tests PASS (existing + new)

- [ ] **Step 7: Run full build**

Run: `go build ./...`
Expected: No compilation errors

- [ ] **Step 8: Commit**

```bash
git add internal/auth/middleware.go internal/auth/store.go internal/auth/auth_test.go internal/server/server.go
git commit -m "feat(auth): add OIDC token sniffing to AuthMiddleware with email-based user lookup"
```

---

### Task 4: Wire OIDC validator in serve command

**Files:**
- Modify: `internal/cli/serve.go`

- [ ] **Step 1: Update serve.go to create OIDCValidator when config is present**

In `internal/cli/serve.go`, after `srv.AuthStore = &auth.Store{DB: database}`, add:

```go
// Wire up OIDC validator if configured.
if cfg.Auth.OIDC.IssuerURL != "" {
	oidcValidator, err := auth.NewOIDCValidator(
		cmd.Context(),
		cfg.Auth.OIDC.IssuerURL,
		cfg.Auth.OIDC.ClientID,
		cfg.Auth.OIDC.Audience,
		cfg.Auth.OIDC.AllowedDomains,
	)
	if err != nil {
		return fmt.Errorf("configuring OIDC: %w", err)
	}
	srv.OIDCValidator = oidcValidator
	fmt.Fprintf(cmd.OutOrStdout(), "OIDC authentication enabled (issuer: %s)\n", cfg.Auth.OIDC.IssuerURL)
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/cli/serve.go
git commit -m "feat(auth): wire OIDC validator into serve command when config present"
```

---

### Task 5: Credential file management

**Files:**
- Create: `internal/cli/credentials.go`
- Create: `internal/cli/credentials_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/credentials_test.go`:

```go
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
		APIKey: "mk_test1234567890",
	}

	if err := SaveCredentials(path, cred); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if loaded.Type != CredTypeAPIKey {
		t.Errorf("type = %q, want %q", loaded.Type, CredTypeAPIKey)
	}
	if loaded.APIKey != "mk_test1234567890" {
		t.Errorf("api_key = %q, want %q", loaded.APIKey, "mk_test1234567890")
	}
}

func TestSaveAndLoadCredentials_OIDC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	expiry := time.Now().Add(time.Hour).Truncate(time.Second)
	cred := &Credentials{
		Type:         CredTypeOIDC,
		AccessToken:  "eyJ.test.token",
		RefreshToken: "refresh-abc",
		ExpiresAt:    expiry,
	}

	if err := SaveCredentials(path, cred); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	loaded, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if loaded.Type != CredTypeOIDC {
		t.Errorf("type = %q, want %q", loaded.Type, CredTypeOIDC)
	}
	if loaded.AccessToken != "eyJ.test.token" {
		t.Errorf("access_token mismatch")
	}
	if loaded.RefreshToken != "refresh-abc" {
		t.Errorf("refresh_token mismatch")
	}
	if !loaded.ExpiresAt.Equal(expiry) {
		t.Errorf("expires_at = %v, want %v", loaded.ExpiresAt, expiry)
	}
}

func TestLoadCredentials_NotFound(t *testing.T) {
	_, err := LoadCredentials("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDeleteCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	cred := &Credentials{Type: CredTypeAPIKey, APIKey: "mk_test"}
	SaveCredentials(path, cred)

	if err := DeleteCredentials(path); err != nil {
		t.Fatalf("DeleteCredentials: %v", err)
	}

	_, err := os.Stat(path)
	if !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestResolveCredentials_Precedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	// Save a cached credential
	SaveCredentials(path, &Credentials{Type: CredTypeAPIKey, APIKey: "mk_cached"})

	// Env var should take precedence over file
	t.Setenv("MANTLE_API_KEY", "mk_from_env")

	resolved, err := ResolveCredentials("", "", path, nil)
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if resolved.Type != CredTypeAPIKey || resolved.APIKey != "mk_from_env" {
		t.Errorf("should prefer env var, got type=%q key=%q", resolved.Type, resolved.APIKey)
	}

	// Flag should take precedence over env var
	resolved, err = ResolveCredentials("mk_from_flag", "", path, nil)
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if resolved.APIKey != "mk_from_flag" {
		t.Errorf("should prefer flag, got %q", resolved.APIKey)
	}
}

func TestResolveCredentials_OIDCEnvVar(t *testing.T) {
	t.Setenv("MANTLE_OIDC_TOKEN", "eyJ.from.env")

	resolved, err := ResolveCredentials("", "", "/nonexistent", nil)
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if resolved.Type != CredTypeOIDC || resolved.AccessToken != "eyJ.from.env" {
		t.Errorf("should use OIDC env var, got type=%q", resolved.Type)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run "TestSaveAndLoad|TestDelete|TestResolve" -v`
Expected: FAIL — types and functions don't exist

- [ ] **Step 3: Implement credential management**

Create `internal/cli/credentials.go`:

```go
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

// Credentials represents cached authentication credentials.
type Credentials struct {
	Type         string    `json:"type"`
	APIKey       string    `json:"api_key,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// DefaultCredentialsPath returns ~/.mantle/credentials.
func DefaultCredentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mantle", "credentials")
}

// SaveCredentials writes credentials to path with 0600 permissions.
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

// LoadCredentials reads credentials from path.
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

// DeleteCredentials removes the credentials file.
func DeleteCredentials(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing credentials: %w", err)
	}
	return nil
}

// ResolveCredentials returns credentials using the precedence chain:
// 1. --api-key flag
// 2. MANTLE_API_KEY env var
// 3. MANTLE_OIDC_TOKEN env var
// 4. ~/.mantle/credentials (with auto-refresh for OIDC)
// 5. Error
//
// The tokenSource parameter enables auto-refresh for expired OIDC tokens.
// Pass nil if no OAuth2 config is available.
func ResolveCredentials(apiKeyFlag, oidcTokenFlag, credPath string, tokenSource oauth2.TokenSource) (*Credentials, error) {
	// 1. --api-key flag
	if apiKeyFlag != "" {
		return &Credentials{Type: CredTypeAPIKey, APIKey: apiKeyFlag}, nil
	}

	// 2. MANTLE_API_KEY env var
	if key := os.Getenv("MANTLE_API_KEY"); key != "" {
		return &Credentials{Type: CredTypeAPIKey, APIKey: key}, nil
	}

	// 3. MANTLE_OIDC_TOKEN env var
	if token := os.Getenv("MANTLE_OIDC_TOKEN"); token != "" {
		return &Credentials{Type: CredTypeOIDC, AccessToken: token}, nil
	}

	// 4. Cached credentials file
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
		if err := SaveCredentials(credPath, cred); err != nil {
			// Non-fatal — we have the token, just can't cache it
			fmt.Fprintf(os.Stderr, "warning: could not save refreshed token: %v\n", err)
		}
	}

	return cred, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run "TestSaveAndLoad|TestDelete|TestResolve" -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/credentials.go internal/cli/credentials_test.go
git commit -m "feat(cli): add credential file management with precedence chain and auto-refresh"
```

---

### Task 6: mantle login and logout commands

**Files:**
- Create: `internal/cli/login.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Implement mantle login**

Create `internal/cli/login.go`:

```go
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"

	"github.com/dvflw/mantle/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCommand() *cobra.Command {
	var (
		useDeviceFlow bool
		useAPIKey     bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Mantle",
		Long:  "Authenticate via OIDC SSO or API key. Credentials are cached at ~/.mantle/credentials.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if useAPIKey {
				return loginAPIKey(cmd)
			}

			cfg := config.FromContext(cmd.Context())
			if cfg == nil || cfg.Auth.OIDC.IssuerURL == "" {
				return fmt.Errorf("OIDC not configured — use 'mantle login --api-key' or configure auth.oidc in mantle.yaml")
			}

			if useDeviceFlow {
				return loginDeviceFlow(cmd, cfg)
			}
			return loginAuthCodePKCE(cmd, cfg)
		},
	}

	cmd.Flags().BoolVar(&useDeviceFlow, "device", false, "Use device authorization flow (for headless/SSH environments)")
	cmd.Flags().BoolVar(&useAPIKey, "api-key", false, "Cache an API key instead of using OIDC")

	return cmd
}

func loginAPIKey(cmd *cobra.Command) error {
	fmt.Fprint(cmd.OutOrStdout(), "Enter API key: ")
	var key string
	fmt.Fscanln(cmd.InOrStdin(), &key)

	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, "mk_") {
		return fmt.Errorf("invalid API key format — keys start with 'mk_'")
	}

	cred := &Credentials{
		Type:   CredTypeAPIKey,
		APIKey: key,
	}

	path := DefaultCredentialsPath()
	if err := SaveCredentials(path, cred); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "API key cached at %s\n", path)
	return nil
}

func loginAuthCodePKCE(cmd *cobra.Command, cfg *config.Config) error {
	// Use OIDC discovery to find endpoints dynamically.
	provider, err := oidc.NewProvider(cmd.Context(), cfg.Auth.OIDC.IssuerURL)
	if err != nil {
		return fmt.Errorf("OIDC discovery failed: %w", err)
	}

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.Auth.OIDC.ClientID,
		ClientSecret: cfg.Auth.OIDC.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  "http://localhost:0/callback",
		Scopes:       []string{"openid", "email", "profile", "offline_access"},
	}

	// Generate PKCE verifier
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return fmt.Errorf("generating PKCE verifier: %w", err)
	}
	verifier := oauth2.S256ChallengeOption(hex.EncodeToString(verifierBytes))

	// Generate state
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	// Start local callback server
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	oauth2Cfg.RedirectURL = fmt.Sprintf("http://localhost:%d/callback", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			fmt.Fprint(w, "Error: state mismatch. Close this tab.")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback: %s", r.URL.Query().Get("error_description"))
			fmt.Fprint(w, "Error: no authorization code received. Close this tab.")
			return
		}
		codeCh <- code
		fmt.Fprint(w, "Authentication successful! You can close this tab.")
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Close()

	// Open browser
	authURL := oauth2Cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, verifier)
	fmt.Fprintf(cmd.OutOrStdout(), "Opening browser to:\n%s\n\n", authURL)
	fmt.Fprintln(cmd.OutOrStdout(), "Waiting for authentication...")

	// Try to open browser (best effort)
	openBrowser(authURL)

	// Wait for callback
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return fmt.Errorf("authentication failed: %w", err)
	case <-cmd.Context().Done():
		return fmt.Errorf("authentication cancelled")
	}

	// Exchange code for token
	token, err := oauth2Cfg.Exchange(cmd.Context(), code, verifier)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	cred := &Credentials{
		Type:         CredTypeOIDC,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}

	path := DefaultCredentialsPath()
	if err := SaveCredentials(path, cred); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Authenticated successfully. Credentials cached at %s\n", path)
	return nil
}

func loginDeviceFlow(cmd *cobra.Command, cfg *config.Config) error {
	// Use OIDC discovery for endpoints. Device flow requires the provider
	// to support device_authorization_endpoint (Okta, Auth0, Azure AD do;
	// Google Workspace does with specific client types).
	provider, err := oidc.NewProvider(cmd.Context(), cfg.Auth.OIDC.IssuerURL)
	if err != nil {
		return fmt.Errorf("OIDC discovery failed: %w", err)
	}

	// Extract device_authorization_endpoint from discovery document
	var providerClaims struct {
		DeviceAuthURL string `json:"device_authorization_endpoint"`
	}
	if err := provider.Claims(&providerClaims); err != nil {
		return fmt.Errorf("parsing provider claims: %w", err)
	}
	if providerClaims.DeviceAuthURL == "" {
		return fmt.Errorf("OIDC provider does not support device authorization flow — use 'mantle login' without --device")
	}

	endpoint := provider.Endpoint()
	endpoint.DeviceAuthURL = providerClaims.DeviceAuthURL

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.Auth.OIDC.ClientID,
		ClientSecret: cfg.Auth.OIDC.ClientSecret,
		Endpoint:     endpoint,
		Scopes:       []string{"openid", "email", "profile", "offline_access"},
	}

	resp, err := oauth2Cfg.DeviceAuth(cmd.Context())
	if err != nil {
		return fmt.Errorf("device authorization request failed: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nTo authenticate, visit:\n  %s\n\nAnd enter code: %s\n\nWaiting...\n",
		resp.VerificationURI, resp.UserCode)

	token, err := oauth2Cfg.DeviceAccessToken(cmd.Context(), resp)
	if err != nil {
		return fmt.Errorf("device flow token exchange failed: %w", err)
	}

	cred := &Credentials{
		Type:         CredTypeOIDC,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}

	path := DefaultCredentialsPath()
	if err := SaveCredentials(path, cred); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Authenticated successfully. Credentials cached at %s\n", path)
	return nil
}

// openBrowser attempts to open the URL in the default browser.
func openBrowser(url string) {
	// Use os/exec to run platform-specific open command.
	// Best effort — if it fails, user can copy the URL.
	//nolint:gosec // URL comes from our own OAuth2 config
	cmd := browserCommand(url)
	if cmd != nil {
		cmd.Start()
	}
}

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove cached credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := DefaultCredentialsPath()
			if err := DeleteCredentials(path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Credentials removed from %s\n", path)
			return nil
		},
	}
}
```

Create `internal/cli/browser_darwin.go`:

```go
//go:build darwin

package cli

import "os/exec"

func browserCommand(url string) *exec.Cmd {
	return exec.Command("open", url)
}
```

Create `internal/cli/browser_linux.go`:

```go
//go:build linux

package cli

import "os/exec"

func browserCommand(url string) *exec.Cmd {
	return exec.Command("xdg-open", url)
}
```

Create `internal/cli/browser_windows.go`:

```go
//go:build windows

package cli

import "os/exec"

func browserCommand(url string) *exec.Cmd {
	return exec.Command("cmd", "/c", "start", url)
}
```

- [ ] **Step 2: Register commands in root.go**

In `internal/cli/root.go`, find where commands are registered and add:

```go
rootCmd.AddCommand(newLoginCommand())
rootCmd.AddCommand(newLogoutCommand())
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/cli/login.go internal/cli/browser_darwin.go internal/cli/browser_linux.go internal/cli/browser_windows.go internal/cli/root.go
git commit -m "feat(cli): add mantle login (auth code PKCE + device flow) and logout commands"
```

---

### Task 7: Add credential flag infrastructure to CLI

**Files:**
- Modify: `internal/cli/root.go`

**Note:** Currently all CLI commands use direct DB access, not HTTP API calls. The credential resolution chain (from Task 5) will be wired into HTTP-based commands when remote CLI mode is added. For now, we add the flag infrastructure so `mantle login` and credential caching work end-to-end.

- [ ] **Step 1: Add a `--api-key` persistent flag to root command**

In `internal/cli/root.go`:

```go
rootCmd.PersistentFlags().String("api-key", "", "API key for authentication (overrides cached credentials)")
```

This makes the flag available to all subcommands for when HTTP-based commands are added.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/root.go
git commit -m "feat(cli): add --api-key persistent flag for credential resolution"
```

---

### Task 8: Integration test — full OIDC auth flow

**Files:**
- Modify: `internal/auth/auth_test.go` or create `internal/auth/oidc_integration_test.go`

- [ ] **Step 1: Write the integration test**

This test verifies the end-to-end flow: mock OIDC server → JWT → middleware → user lookup in real Postgres.

```go
func TestOIDCIntegration_FullFlow(t *testing.T) {
	// Set up real Postgres via testcontainers
	store := setupTestStore(t)
	ctx := context.Background()

	// Pre-provision user
	_, err := store.CreateUser(ctx, "alice@example.com", "Alice", DefaultTeamID, RoleOperator)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Set up mock OIDC server
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	oidcSrv := mockOIDCServer(t, key)

	validator, err := NewOIDCValidator(ctx, oidcSrv.URL, "test-client", "mantle", []string{"example.com"})
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	// Create test HTTP handler behind auth middleware
	var capturedUser *User
	var capturedMethod string
	appHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		capturedMethod = AuthMethodFromContext(r.Context())
		w.WriteHeader(200)
	})

	handler := AuthMiddleware(store, validator, appHandler)

	// Test OIDC auth
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            oidcSrv.URL,
		"aud":            "mantle",
		"sub":            "user-123",
		"email":          "alice@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/api/v1/workflows", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("OIDC auth: status = %d, want 200", rec.Code)
	}
	if capturedUser == nil || capturedUser.Email != "alice@example.com" {
		t.Errorf("OIDC auth: user not correctly resolved")
	}
	if capturedMethod != "oidc" {
		t.Errorf("auth_method = %q, want %q", capturedMethod, "oidc")
	}

	// Test API key auth still works alongside OIDC
	apiUser, _ := store.CreateUser(ctx, "bob@example.com", "Bob", DefaultTeamID, RoleAdmin)
	rawKey, _, _ := store.CreateAPIKey(ctx, apiUser.ID, "test-key")

	req2 := httptest.NewRequest("GET", "/api/v1/workflows", nil)
	req2.Header.Set("Authorization", "Bearer "+rawKey)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != 200 {
		t.Fatalf("API key auth: status = %d, want 200", rec2.Code)
	}
	if capturedMethod != "api_key" {
		t.Errorf("auth_method = %q, want %q", capturedMethod, "api_key")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/auth/ -run TestOIDCIntegration -v -timeout 120s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/auth/auth_test.go
git commit -m "test(auth): add end-to-end OIDC integration test with real Postgres"
```

---

### Task 9: Audit method tracking

**Files:**
- Modify: `internal/auth/middleware.go` (already done in Task 3 — `withAuthMethod`/`AuthMethodFromContext`)
- Verify: audit emitter picks up auth method from context

- [ ] **Step 1: Check how audit events access context**

Read `internal/audit/` to understand how audit events are emitted and whether they access request context. If the emitter already logs context values, `AuthMethodFromContext` will be picked up automatically.

If not, add `auth_method` to the audit event metadata wherever state-changing operations emit events.

- [ ] **Step 2: Verify build and tests**

Run: `go build ./... && go test ./internal/auth/ -v -timeout 120s`
Expected: All pass

- [ ] **Step 3: Commit if changes were needed**

```bash
git add internal/audit/
git commit -m "feat(audit): include auth_method in audit event context"
```

---

## Summary

| Task | Description | Deps |
|------|-------------|------|
| 1 | OIDC config structs + env vars | None |
| 2 | OIDC JWT validator (`oidc.go`) | 1 (uses config types) |
| 3 | Token-sniffing middleware | 2 (uses validator) |
| 4 | Wire validator in serve command | 1, 2, 3 |
| 5 | Credential file management | None (independent) |
| 6 | `mantle login` / `mantle logout` | 5 (uses credentials) |
| 7 | Wire credential resolution | 5, 6 |
| 8 | Integration test | 2, 3 |
| 9 | Audit method tracking | 3 |

Tasks 1-4 are the server-side chain. Tasks 5-7 are the CLI chain. These two chains can be executed in parallel. Task 8 ties them together. Task 9 is a quick verification.
