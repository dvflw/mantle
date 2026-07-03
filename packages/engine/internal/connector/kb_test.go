package connector

import (
	"context"
	"strings"
	"testing"
)

// A malformed connection string so pgx.Connect fails immediately at parse time
// (no network), letting us assert the connector didn't mutate its params.
const kbBadCredURL = "postgres://bad host"

// The engine resolves a step's params once and reuses that map across retry
// attempts, so a connector must not mutate it — deleting _credential would make
// attempt 2 fail with a missing credential instead of retrying.
func TestKBUpsert_DoesNotMutateCredential(t *testing.T) {
	params := map[string]any{
		"_credential": map[string]string{"url": kbBadCredURL},
		"table":       "kb_documents",
		"content":     "x",
		"vector":      "[1]",
	}
	if _, err := (&KBUpsertConnector{}).Execute(context.Background(), params); err == nil {
		t.Fatal("expected a connection error")
	}
	if _, ok := params["_credential"]; !ok {
		t.Error("_credential was removed from params (would break retries)")
	}
}

func TestKBQuery_DoesNotMutateCredential(t *testing.T) {
	params := map[string]any{
		"_credential": map[string]string{"url": kbBadCredURL},
		"table":       "kb_documents",
		"vector":      "[1]",
	}
	if _, err := (&KBQueryConnector{}).Execute(context.Background(), params); err == nil {
		t.Fatal("expected a connection error")
	}
	if _, ok := params["_credential"]; !ok {
		t.Error("_credential was removed from params (would break retries)")
	}
}

func TestPrepareUpsert_SingleRow(t *testing.T) {
	sql, args, err := prepareUpsert(map[string]any{
		"table":   "kb_documents",
		"content": "hello",
		"vector":  "[0.1,0.2]",
	})
	if err != nil {
		t.Fatalf("prepareUpsert error: %v", err)
	}
	if !strings.Contains(sql, "INSERT INTO kb_documents (content, embedding)") {
		t.Errorf("unexpected sql: %s", sql)
	}
	if !strings.Contains(sql, "e::vector") || !strings.Contains(sql, "unnest($1::text[], $2::text[])") {
		t.Errorf("missing unnest/cast: %s", sql)
	}
	if len(args) != 2 {
		t.Fatalf("args len = %d, want 2", len(args))
	}
	contents := args[0].([]string)
	vectors := args[1].([]string)
	if len(contents) != 1 || contents[0] != "hello" || vectors[0] != "[0.1,0.2]" {
		t.Errorf("args = %v / %v", contents, vectors)
	}
}

func TestPrepareUpsert_BatchWithMetadataAndConflict(t *testing.T) {
	sql, args, err := prepareUpsert(map[string]any{
		"table":            "kb_documents",
		"contents":         []any{"a", "b"},
		"vectors":          []any{"[1]", "[2]"},
		"metadatas":        []any{map[string]any{"src": "x"}, map[string]any{"src": "y"}},
		"metadata_column":  "meta",
		"conflict_target":  "dedupe_key",
		"embedding_column": "emb",
	})
	if err != nil {
		t.Fatalf("prepareUpsert error: %v", err)
	}
	if !strings.Contains(sql, "INSERT INTO kb_documents (content, emb, meta)") {
		t.Errorf("unexpected columns: %s", sql)
	}
	if !strings.Contains(sql, "m::jsonb") || !strings.Contains(sql, "unnest($1::text[], $2::text[], $3::text[])") {
		t.Errorf("missing metadata handling: %s", sql)
	}
	if !strings.HasSuffix(sql, "ON CONFLICT (dedupe_key) DO NOTHING") {
		t.Errorf("missing on conflict: %s", sql)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
	metas := args[2].([]string)
	if len(metas) != 2 || !strings.Contains(metas[0], `"src":"x"`) {
		t.Errorf("metadata args = %v", metas)
	}
}

func TestPrepareUpsert_SingleMetadataBroadcast(t *testing.T) {
	// One `metadata` object applies to every row of a batch (chunked ingest).
	_, args, err := prepareUpsert(map[string]any{
		"table":    "kb_documents",
		"contents": []any{"a", "b", "c"},
		"vectors":  []any{"[1]", "[2]", "[3]"},
		"metadata": map[string]any{"source": "doc-1"},
	})
	if err != nil {
		t.Fatalf("prepareUpsert error: %v", err)
	}
	metas := args[2].([]string)
	if len(metas) != 3 {
		t.Fatalf("metadata args = %d, want 3 (broadcast)", len(metas))
	}
	for i, m := range metas {
		if !strings.Contains(m, `"source":"doc-1"`) {
			t.Errorf("metas[%d] = %q, want broadcast source", i, m)
		}
	}
}

func TestPrepareUpsert_Errors(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
	}{
		{"missing table", map[string]any{"content": "x", "vector": "[1]"}},
		{"missing content", map[string]any{"table": "t", "vector": "[1]"}},
		{"both content forms", map[string]any{"table": "t", "content": "x", "contents": []any{"y"}, "vector": "[1]"}},
		{"count mismatch", map[string]any{"table": "t", "contents": []any{"a", "b"}, "vectors": []any{"[1]"}}},
		{"metadata mismatch", map[string]any{"table": "t", "content": "a", "vector": "[1]", "metadatas": []any{map[string]any{}, map[string]any{}}}},
		{"both metadata forms", map[string]any{"table": "t", "content": "a", "vector": "[1]", "metadata": map[string]any{}, "metadatas": []any{map[string]any{}}}},
		{"both vector forms", map[string]any{"table": "t", "content": "a", "vector": "[1]", "vectors": []any{"[1]"}}},
		{"sql injection in table", map[string]any{"table": "kb; DROP TABLE users;--", "content": "x", "vector": "[1]"}},
		{"sql injection in column", map[string]any{"table": "t", "content_column": "c)); DROP", "content": "x", "vector": "[1]"}},
		{"sql injection in conflict", map[string]any{"table": "t", "content": "x", "vector": "[1]", "conflict_target": "a) DO UPDATE"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := prepareUpsert(tc.params); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestPrepareQuery_Default(t *testing.T) {
	sql, args, err := prepareQuery(map[string]any{
		"table":  "kb_documents",
		"vector": "[0.1,0.2]",
	})
	if err != nil {
		t.Fatalf("prepareQuery error: %v", err)
	}
	// Default: cosine operator, content column, top_k 5.
	if !strings.Contains(sql, "SELECT content, (embedding <=> $1::vector) AS distance") {
		t.Errorf("unexpected select: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY embedding <=> $1::vector LIMIT 5") {
		t.Errorf("unexpected order/limit: %s", sql)
	}
	if len(args) != 1 || args[0].(string) != "[0.1,0.2]" {
		t.Errorf("args = %v", args)
	}
}

func TestPrepareQuery_MetricsAndColumns(t *testing.T) {
	sql, _, err := prepareQuery(map[string]any{
		"table":            "kb_documents",
		"vector":           "[1]",
		"metric":           "l2",
		"columns":          []any{"title", "content", "source"},
		"embedding_column": "emb",
		"top_k":            3,
	})
	if err != nil {
		t.Fatalf("prepareQuery error: %v", err)
	}
	if !strings.Contains(sql, "SELECT title, content, source, (emb <-> $1::vector) AS distance") {
		t.Errorf("unexpected select: %s", sql)
	}
	if !strings.HasSuffix(sql, "LIMIT 3") {
		t.Errorf("unexpected limit: %s", sql)
	}
}

func TestPrepareQuery_TopKCap(t *testing.T) {
	sql, _, err := prepareQuery(map[string]any{"table": "t", "vector": "[1]", "top_k": 999999})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.HasSuffix(sql, "LIMIT 1000") {
		t.Errorf("top_k not capped: %s", sql)
	}
}

func TestPrepareQuery_Errors(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
	}{
		{"missing table", map[string]any{"vector": "[1]"}},
		{"missing vector", map[string]any{"table": "t"}},
		{"bad metric", map[string]any{"table": "t", "vector": "[1]", "metric": "manhattan"}},
		{"negative top_k", map[string]any{"table": "t", "vector": "[1]", "top_k": -1}},
		{"zero top_k", map[string]any{"table": "t", "vector": "[1]", "top_k": 0}},
		{"sql injection in table", map[string]any{"table": "t; DROP TABLE x", "vector": "[1]"}},
		{"sql injection in column", map[string]any{"table": "t", "vector": "[1]", "columns": []any{"content", "x); DROP"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := prepareQuery(tc.params); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}
