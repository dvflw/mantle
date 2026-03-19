package secret

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
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

	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	return &Store{DB: database, Encryptor: enc}
}

func TestStore_CreateAndGet(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	cred, err := store.Create(ctx, "my-openai", "openai", map[string]string{
		"api_key": "sk-test123",
		"org_id":  "org-456",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if cred.Name != "my-openai" {
		t.Errorf("name = %q, want %q", cred.Name, "my-openai")
	}
	if cred.Type != "openai" {
		t.Errorf("type = %q, want %q", cred.Type, "openai")
	}

	// Retrieve and decrypt.
	data, err := store.Get(ctx, "my-openai")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if data["api_key"] != "sk-test123" {
		t.Errorf("api_key = %q, want %q", data["api_key"], "sk-test123")
	}
	if data["org_id"] != "org-456" {
		t.Errorf("org_id = %q, want %q", data["org_id"], "org-456")
	}
}

func TestStore_Create_ValidationError(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, "bad", "openai", map[string]string{})
	if err == nil {
		t.Error("Create() with missing required field should fail")
	}
}

func TestStore_Create_UnknownType(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, "bad", "nonexistent", map[string]string{"key": "val"})
	if err == nil {
		t.Error("Create() with unknown type should fail")
	}
}

func TestStore_List(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	store.Create(ctx, "cred-a", "generic", map[string]string{"key": "val-a"})
	store.Create(ctx, "cred-b", "bearer", map[string]string{"token": "tok-b"})

	creds, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("List() returned %d, want 2", len(creds))
	}
	// Sorted by name.
	if creds[0].Name != "cred-a" {
		t.Errorf("first credential = %q, want %q", creds[0].Name, "cred-a")
	}
}

func TestStore_Delete(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	store.Create(ctx, "to-delete", "generic", map[string]string{"key": "val"})

	err := store.Delete(ctx, "to-delete")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = store.Get(ctx, "to-delete")
	if err == nil {
		t.Error("Get() after delete should fail")
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	if err == nil {
		t.Error("Delete() nonexistent should fail")
	}
}

func TestStore_ReEncryptAll(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	store.Create(ctx, "cred-1", "generic", map[string]string{"key": "secret-1"})
	store.Create(ctx, "cred-2", "bearer", map[string]string{"token": "secret-2"})

	// Create a new encryptor with a different key.
	newEnc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	count, err := store.ReEncryptAll(ctx, newEnc)
	if err != nil {
		t.Fatalf("ReEncryptAll() error: %v", err)
	}
	if count != 2 {
		t.Errorf("ReEncryptAll() count = %d, want 2", count)
	}

	// Old encryptor should no longer work.
	_, err = store.Get(ctx, "cred-1")
	if err == nil {
		t.Error("Get() with old encryptor should fail after re-encryption")
	}

	// Switch to new encryptor and verify.
	store.Encryptor = newEnc
	data, err := store.Get(ctx, "cred-1")
	if err != nil {
		t.Fatalf("Get() with new encryptor error: %v", err)
	}
	if data["key"] != "secret-1" {
		t.Errorf("key = %q, want %q", data["key"], "secret-1")
	}
}

func TestResolver_EnvFallback(t *testing.T) {
	resolver := &Resolver{Store: nil}

	t.Setenv("MANTLE_SECRET_MY_API_KEY", "env-secret-value")

	data, err := resolver.Resolve(context.Background(), "my-api-key")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if data["key"] != "env-secret-value" {
		t.Errorf("key = %q, want %q", data["key"], "env-secret-value")
	}
}

func TestResolver_NotFound(t *testing.T) {
	resolver := &Resolver{Store: nil}

	_, err := resolver.Resolve(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Resolve() should fail for missing credential")
	}
}

// Verify raw data in Postgres is not plaintext.
func TestStore_DataEncryptedAtRest(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	store.Create(ctx, "encrypted-check", "generic", map[string]string{"key": "supersecret"})

	var rawData []byte
	err := store.DB.QueryRowContext(ctx,
		`SELECT encrypted_data FROM credentials WHERE name = $1`, "encrypted-check",
	).Scan(&rawData)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}

	// Raw bytes should not contain the plaintext value.
	if containsBytes(rawData, []byte("supersecret")) {
		t.Error("encrypted_data contains plaintext — encryption is not working")
	}
}

func containsBytes(haystack, needle []byte) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Ensure the store variable satisfies the need for *sql.DB (compile check).
var _ *sql.DB
