package connector

import (
	"context"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/dbdefaults"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupExternalPG spins up a throwaway Postgres via testcontainers and returns
// the connection URL. The container is terminated when the test finishes.
func setupExternalPG(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := tcpostgres.Run(ctx,
		dbdefaults.PostgresImage,
		tcpostgres.WithDatabase("ext_test"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skipf("Could not start Postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	})
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Create a test table with seed data.
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		t.Fatalf("Failed to connect for setup: %v", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		);
		INSERT INTO users (name, email) VALUES
			('Alice', 'alice@example.com'),
			('Bob', 'bob@example.com'),
			('Charlie', 'charlie@example.com');
	`)
	if err != nil {
		t.Fatalf("Failed to seed test table: %v", err)
	}

	return connStr
}

func TestPostgresQuery_Select(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	output, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "SELECT id, name, email FROM users ORDER BY id",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	rows, ok := output["rows"].([]map[string]any)
	if !ok {
		t.Fatalf("rows type = %T, want []map[string]any", output["rows"])
	}
	if len(rows) != 3 {
		t.Fatalf("row_count = %d, want 3", len(rows))
	}
	if output["row_count"] != int64(3) {
		t.Errorf("row_count = %v, want 3", output["row_count"])
	}

	if rows[0]["name"] != "Alice" {
		t.Errorf("rows[0][name] = %v, want Alice", rows[0]["name"])
	}
	if rows[1]["email"] != "bob@example.com" {
		t.Errorf("rows[1][email] = %v, want bob@example.com", rows[1]["email"])
	}
}

func TestPostgresQuery_SelectWithArgs(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	output, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "SELECT id, name FROM users WHERE name = $1",
		"args":        []any{"Bob"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	rows := output["rows"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("row_count = %d, want 1", len(rows))
	}
	if rows[0]["name"] != "Bob" {
		t.Errorf("rows[0][name] = %v, want Bob", rows[0]["name"])
	}
}

func TestPostgresQuery_Insert(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	output, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "INSERT INTO users (name, email) VALUES ($1, $2)",
		"args":        []any{"Dave", "dave@example.com"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	affected, ok := output["rows_affected"].(int64)
	if !ok {
		t.Fatalf("rows_affected type = %T, want int64", output["rows_affected"])
	}
	if affected != 1 {
		t.Errorf("rows_affected = %d, want 1", affected)
	}

	// Verify the row was actually inserted.
	verify, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "SELECT name FROM users WHERE email = $1",
		"args":        []any{"dave@example.com"},
	})
	if err != nil {
		t.Fatalf("Verify SELECT error: %v", err)
	}
	rows := verify["rows"].([]map[string]any)
	if len(rows) != 1 || rows[0]["name"] != "Dave" {
		t.Errorf("inserted row not found or wrong name: %v", rows)
	}
}

func TestPostgresQuery_Update(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	output, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "UPDATE users SET email = $1 WHERE name = $2",
		"args":        []any{"alice-new@example.com", "Alice"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if output["rows_affected"] != int64(1) {
		t.Errorf("rows_affected = %v, want 1", output["rows_affected"])
	}
}

func TestPostgresQuery_Delete(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	output, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "DELETE FROM users WHERE name = $1",
		"args":        []any{"Charlie"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if output["rows_affected"] != int64(1) {
		t.Errorf("rows_affected = %v, want 1", output["rows_affected"])
	}
}

func TestPostgresQuery_EmptyResult(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	output, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "SELECT * FROM users WHERE name = $1",
		"args":        []any{"Nobody"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	rows := output["rows"].([]map[string]any)
	if len(rows) != 0 {
		t.Errorf("expected empty result, got %d rows", len(rows))
	}
	if output["row_count"] != int64(0) {
		t.Errorf("row_count = %v, want 0", output["row_count"])
	}
}

func TestPostgresQuery_CredentialKeyField(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	// Use "key" instead of "url" in credential.
	output, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"key": connURL},
		"query":       "SELECT count(*) AS cnt FROM users",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	rows := output["rows"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestPostgresQuery_MissingQuery(t *testing.T) {
	c := &PostgresQueryConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": "postgres://localhost/test"},
	})
	if err == nil {
		t.Fatal("expected error for missing query, got nil")
	}
}

func TestPostgresQuery_MissingCredential(t *testing.T) {
	c := &PostgresQueryConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential, got nil")
	}
}

func TestPostgresQuery_InvalidURL(t *testing.T) {
	c := &PostgresQueryConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": "postgres://invalid:5432/nope?connect_timeout=1"},
		"query":       "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestPostgresQuery_BadSQL(t *testing.T) {
	connURL := setupExternalPG(t)
	c := &PostgresQueryConnector{}

	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"url": connURL},
		"query":       "SELECT * FROM nonexistent_table",
	})
	if err == nil {
		t.Fatal("expected error for bad SQL, got nil")
	}
}

func TestPostgresQuery_RegistryRegistered(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("postgres/query")
	if err != nil {
		t.Fatalf("postgres/query not found in registry: %v", err)
	}
}
