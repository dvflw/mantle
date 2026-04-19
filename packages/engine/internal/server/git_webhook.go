package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/repo"
	reposync "github.com/dvflw/mantle/internal/repo/sync"
)

// GitWebhookHandler serves POST /hooks/git/<repo-id>. It verifies the
// HMAC-SHA256 signature using the repo's webhook_secret, emits a
// git.push.received audit event, and fires SyncRepo in a goroutine so
// the webhook responds within the git provider's timeout.
type GitWebhookHandler struct {
	DB     *sql.DB
	Store  *repo.Store
	Driver reposync.Driver
}

func (h *GitWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/hooks/git/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, `{"error":"bad repo id"}`, http.StatusBadRequest)
		return
	}

	repoRow, err := h.Store.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"repo not found"}`, http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"bad body"}`, http.StatusBadRequest)
		return
	}

	if repoRow.WebhookSecret != "" {
		if !verifyGitWebhookSignature(body, repoRow.WebhookSecret, r.Header) {
			http.Error(w, `{"error":"invalid signature"}`, http.StatusForbidden)
			return
		}
	}

	emitGitAudit(context.Background(), h.DB, audit.Event{
		Timestamp: time.Now(),
		Actor:     "webhook",
		Action:    audit.ActionGitPushReceived,
		Resource:  audit.Resource{Type: "git_repo", ID: repoRow.ID},
		Metadata:  map[string]string{"name": repoRow.Name},
	})

	// Fire sync in the background so the provider gets its 202 fast.
	go func() {
		_, _ = reposync.SyncRepo(context.Background(), h.DB, h.Store, repoRow, h.Driver)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"repo":"%s","accepted":true}`, repoRow.Name)
}

// verifyGitWebhookSignature accepts X-Hub-Signature-256 (GitHub) or
// X-Signature-256 (generic). Returns false if absent or mismatched.
// Kept separate from the workflow webhook's verifier so either can
// change signature format without breaking the other.
func verifyGitWebhookSignature(body []byte, secret string, headers http.Header) bool {
	sigHeader := headers.Get("X-Hub-Signature-256")
	if sigHeader == "" {
		sigHeader = headers.Get("X-Signature-256")
	}
	if sigHeader == "" || !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(sigHeader[len("sha256="):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sig, mac.Sum(nil))
}

// emitGitAudit opens a short-lived transaction for the audit write and
// swallows errors — an audit emission failure must never fail a
// webhook response. Named distinctly from the sync package's emit() so
// linters don't flag the duplication.
func emitGitAudit(ctx context.Context, database *sql.DB, e audit.Event) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback() //nolint:errcheck
	if err := audit.EmitTx(ctx, tx, e); err != nil {
		return
	}
	_ = tx.Commit()
}
