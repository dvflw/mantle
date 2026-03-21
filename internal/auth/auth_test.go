package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mantle_test"),
		postgres.WithUsername("mantle"),
		postgres.WithPassword("mantle"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skipf("Could not start Postgres container: %v", err)
	}
	t.Cleanup(func() { pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}
	database, err := db.Open(config.DatabaseConfig{URL: connStr})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	return &Store{DB: database}
}

func TestTeamCRUD(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	team, err := store.CreateTeam(ctx, "test-team")
	if err != nil {
		t.Fatalf("CreateTeam() error: %v", err)
	}
	if team.Name != "test-team" {
		t.Errorf("name = %q, want %q", team.Name, "test-team")
	}

	teams, err := store.ListTeams(ctx)
	if err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	// Should include default + test-team
	if len(teams) < 2 {
		t.Errorf("ListTeams() returned %d, want >= 2", len(teams))
	}

	err = store.DeleteTeam(ctx, "test-team")
	if err != nil {
		t.Fatalf("DeleteTeam() error: %v", err)
	}
}

func TestDeleteDefaultTeam_Fails(t *testing.T) {
	store := setupTestStore(t)
	err := store.DeleteTeam(context.Background(), "default")
	if err == nil {
		t.Error("DeleteTeam(default) should fail")
	}
}

func TestUserCRUD(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "alice@example.com", "Alice", DefaultTeamID, RoleOperator)
	if err != nil {
		t.Fatalf("CreateUser() error: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", user.Email, "alice@example.com")
	}
	if user.Role != RoleOperator {
		t.Errorf("role = %q, want %q", user.Role, RoleOperator)
	}

	users, err := store.ListUsers(ctx, DefaultTeamID)
	if err != nil {
		t.Fatalf("ListUsers() error: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("ListUsers() returned %d, want 1", len(users))
	}

	err = store.SetUserRole(ctx, "alice@example.com", DefaultTeamID, RoleTeamOwner)
	if err != nil {
		t.Fatalf("SetUserRole() error: %v", err)
	}

	err = store.DeleteUser(ctx, "alice@example.com", DefaultTeamID)
	if err != nil {
		t.Fatalf("DeleteUser() error: %v", err)
	}
}

func TestAPIKeyFlow(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	user, _ := store.CreateUser(ctx, "bob@example.com", "Bob", DefaultTeamID, RoleAdmin)

	rawKey, key, err := store.CreateAPIKey(ctx, user.ID, "test-key")
	if err != nil {
		t.Fatalf("CreateAPIKey() error: %v", err)
	}
	if rawKey == "" || key == nil {
		t.Fatal("CreateAPIKey() returned empty key")
	}
	if key.KeyPrefix != rawKey[:10] {
		t.Errorf("prefix = %q, want %q", key.KeyPrefix, rawKey[:10])
	}

	// Lookup by raw key.
	foundUser, err := store.LookupAPIKey(ctx, rawKey)
	if err != nil {
		t.Fatalf("LookupAPIKey() error: %v", err)
	}
	if foundUser == nil {
		t.Fatal("LookupAPIKey() returned nil user")
	}
	if foundUser.Email != "bob@example.com" {
		t.Errorf("email = %q, want %q", foundUser.Email, "bob@example.com")
	}

	// Invalid key.
	notFound, err := store.LookupAPIKey(ctx, "mk_invalid_key_value")
	if err != nil {
		t.Fatalf("LookupAPIKey() error: %v", err)
	}
	if notFound != nil {
		t.Error("LookupAPIKey() should return nil for invalid key")
	}
}

func TestHashAPIKey(t *testing.T) {
	h1 := HashAPIKey("mk_test123")
	h2 := HashAPIKey("mk_test123")
	h3 := HashAPIKey("mk_different")

	if h1 != h2 {
		t.Error("same key should produce same hash")
	}
	if h1 == h3 {
		t.Error("different keys should produce different hashes")
	}
}

func TestHasMinRole(t *testing.T) {
	tests := []struct {
		userRole Role
		minRole  Role
		want     bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleTeamOwner, true},
		{RoleAdmin, RoleOperator, true},
		{RoleTeamOwner, RoleAdmin, false},
		{RoleTeamOwner, RoleTeamOwner, true},
		{RoleTeamOwner, RoleOperator, true},
		{RoleOperator, RoleAdmin, false},
		{RoleOperator, RoleTeamOwner, false},
		{RoleOperator, RoleOperator, true},
	}
	for _, tt := range tests {
		got := hasMinRole(tt.userRole, tt.minRole)
		if got != tt.want {
			t.Errorf("hasMinRole(%q, %q) = %v, want %v", tt.userRole, tt.minRole, got, tt.want)
		}
	}
}

func TestAuthMiddleware_NoHeader(t *testing.T) {
	handler := AuthMiddleware(&Store{}, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without auth")
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_HealthBypass(t *testing.T) {
	called := false
	handler := AuthMiddleware(&Store{}, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("health endpoint should bypass auth")
	}
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddleware_OIDCToken(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Pre-provision user.
	_, err := store.CreateUser(ctx, "alice@example.com", "Alice", DefaultTeamID, RoleOperator)
	if err != nil {
		t.Fatalf("CreateUser() error: %v", err)
	}

	// Set up mock OIDC server.
	key, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	srv := mockOIDCServer(t, key)

	validator, err := NewOIDCValidator(ctx, srv.URL, srv.URL, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewOIDCValidator() error: %v", err)
	}

	// Sign a JWT with matching email.
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            srv.URL,
		"sub":            "user-123",
		"email":          "alice@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	var capturedUser *User
	var capturedMethod string
	handler := AuthMiddleware(store, validator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		capturedMethod = AuthMethodFromContext(r.Context())
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if capturedUser == nil {
		t.Fatal("user should be set in context")
	}
	if capturedUser.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", capturedUser.Email, "alice@example.com")
	}
	if capturedMethod != "oidc" {
		t.Errorf("auth method = %q, want %q", capturedMethod, "oidc")
	}
}

func TestAuthMiddleware_OIDCToken_UserNotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	key, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	srv := mockOIDCServer(t, key)

	validator, err := NewOIDCValidator(ctx, srv.URL, srv.URL, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewOIDCValidator() error: %v", err)
	}

	// Sign a JWT for a user that does NOT exist in the database.
	token := signTestJWT(t, key, jwt.MapClaims{
		"iss":            srv.URL,
		"aud":            srv.URL,
		"sub":            "user-999",
		"email":          "nobody@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})

	handler := AuthMiddleware(store, validator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for unknown user")
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestAuthMiddleware_APIKeyStillWorks(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "bob@example.com", "Bob", DefaultTeamID, RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser() error: %v", err)
	}

	rawKey, _, err := store.CreateAPIKey(ctx, user.ID, "test-key")
	if err != nil {
		t.Fatalf("CreateAPIKey() error: %v", err)
	}

	var capturedUser *User
	var capturedMethod string
	// Pass nil for oidcValidator — API keys should still work.
	handler := AuthMiddleware(store, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		capturedMethod = AuthMethodFromContext(r.Context())
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if capturedUser == nil {
		t.Fatal("user should be set in context")
	}
	if capturedUser.Email != "bob@example.com" {
		t.Errorf("email = %q, want %q", capturedUser.Email, "bob@example.com")
	}
	if capturedMethod != "api_key" {
		t.Errorf("auth method = %q, want %q", capturedMethod, "api_key")
	}
}

func TestOIDCIntegration_FullFlow(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// 1. Pre-provision alice (OIDC user).
	_, err := store.CreateUser(ctx, "alice@example.com", "Alice", DefaultTeamID, RoleOperator)
	if err != nil {
		t.Fatalf("CreateUser(alice) error: %v", err)
	}

	// 2. Pre-provision bob (API key user) and create an API key.
	bob, err := store.CreateUser(ctx, "bob@example.com", "Bob", DefaultTeamID, RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser(bob) error: %v", err)
	}
	rawKey, _, err := store.CreateAPIKey(ctx, bob.ID, "bob-key")
	if err != nil {
		t.Fatalf("CreateAPIKey() error: %v", err)
	}

	// 3. Set up mock OIDC server with a test RSA key.
	key, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	srv := mockOIDCServer(t, key)

	// 4. Create an OIDCValidator pointing at the mock server.
	validator, err := NewOIDCValidator(ctx, srv.URL, srv.URL, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewOIDCValidator() error: %v", err)
	}

	// 5. Create a single handler behind AuthMiddleware that captures user and auth method.
	var capturedUser *User
	var capturedMethod string
	handler := AuthMiddleware(store, validator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		capturedMethod = AuthMethodFromContext(r.Context())
		w.WriteHeader(200)
	}))

	// 6. OIDC path: sign a JWT for alice and send it.
	t.Run("oidc_path", func(t *testing.T) {
		capturedUser = nil
		capturedMethod = ""

		token := signTestJWT(t, key, jwt.MapClaims{
			"iss":            srv.URL,
			"aud":            srv.URL,
			"sub":            "alice-oidc-sub",
			"email":          "alice@example.com",
			"email_verified": true,
			"exp":            time.Now().Add(time.Hour).Unix(),
			"iat":            time.Now().Unix(),
		})

		req := httptest.NewRequest("GET", "/api/workflows", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != 200 {
			t.Fatalf("OIDC path: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
		}
		if capturedUser == nil {
			t.Fatal("OIDC path: user should be set in context")
		}
		if capturedUser.Email != "alice@example.com" {
			t.Errorf("OIDC path: email = %q, want %q", capturedUser.Email, "alice@example.com")
		}
		if capturedMethod != "oidc" {
			t.Errorf("OIDC path: auth method = %q, want %q", capturedMethod, "oidc")
		}
	})

	// 7. API key path: send bob's API key through the same middleware.
	t.Run("api_key_path", func(t *testing.T) {
		capturedUser = nil
		capturedMethod = ""

		req := httptest.NewRequest("GET", "/api/workflows", nil)
		req.Header.Set("Authorization", "Bearer "+rawKey)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != 200 {
			t.Fatalf("API key path: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
		}
		if capturedUser == nil {
			t.Fatal("API key path: user should be set in context")
		}
		if capturedUser.Email != "bob@example.com" {
			t.Errorf("API key path: email = %q, want %q", capturedUser.Email, "bob@example.com")
		}
		if capturedMethod != "api_key" {
			t.Errorf("API key path: auth method = %q, want %q", capturedMethod, "api_key")
		}
	})
}

// compile check
var _ *sql.DB
