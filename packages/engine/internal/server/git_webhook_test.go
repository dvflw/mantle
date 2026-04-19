package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/dbdefaults"
	"github.com/dvflw/mantle/internal/repo"
	reposync "github.com/dvflw/mantle/internal/repo/sync"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupWebhookTest(t *testing.T) (*sql.DB, *repo.Store) {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx,
		dbdefaults.PostgresImage,
		postgres.WithDatabase(dbdefaults.TestDatabase),
		postgres.WithUsername(dbdefaults.User),
		postgres.WithPassword(dbdefaults.Password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("Postgres (CI): %v", err)
		}
		t.Skipf("Postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	connStr, _ := pg.ConnectionString(ctx, "sslmode=disable")
	database, err := db.Open(config.DatabaseConfig{URL: connStr})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return database, &repo.Store{DB: database, Actor: "test"}
}

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestGitWebhook_ValidSignature_202(t *testing.T) {
	database, store := setupWebhookTest(t)
	ctx := context.Background()

	// Create a repo with a webhook_secret directly (Plan A doesn't expose
	// a setter; write the column via SQL).
	r, err := store.Create(ctx, repo.CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	secret := "topsecret"
	_, err = database.ExecContext(ctx,
		`UPDATE git_repos SET webhook_secret = $1 WHERE id = $2`, secret, r.ID)
	if err != nil {
		t.Fatalf("set webhook_secret: %v", err)
	}

	h := &GitWebhookHandler{
		DB:     database,
		Store:  store,
		Driver: &reposync.NoopDriver{BasePath: t.TempDir(), SHA: "sha-a"},
	}
	body := []byte(`{"hello":"world"}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/git/"+r.ID, bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, secret))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want 202; body: %s", rec.Code, rec.Body.String())
	}
	if want := `"accepted":true`; !bytes.Contains(rec.Body.Bytes(), []byte(want)) {
		t.Errorf("body missing %q: %s", want, rec.Body.String())
	}
}

func TestGitWebhook_BadSignature_403(t *testing.T) {
	database, store := setupWebhookTest(t)
	ctx := context.Background()
	r, _ := store.Create(ctx, repo.CreateParams{
		Name: "acme2", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	_, _ = database.ExecContext(ctx,
		`UPDATE git_repos SET webhook_secret = 'real' WHERE id = $1`, r.ID)

	h := &GitWebhookHandler{DB: database, Store: store, Driver: &reposync.NoopDriver{BasePath: t.TempDir()}}
	req := httptest.NewRequest(http.MethodPost, "/hooks/git/"+r.ID, bytes.NewReader([]byte("x")))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGitWebhook_UnknownRepo_404(t *testing.T) {
	database, store := setupWebhookTest(t)
	h := &GitWebhookHandler{DB: database, Store: store, Driver: &reposync.NoopDriver{BasePath: t.TempDir()}}
	req := httptest.NewRequest(http.MethodPost, "/hooks/git/00000000-0000-0000-0000-000000000000", bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}
