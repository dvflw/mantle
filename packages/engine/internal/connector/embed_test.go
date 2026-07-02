package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// embedTestServer returns an httptest server that emits `n` deterministic
// embeddings echoing the request order via the index field.
func embedTestServer(t *testing.T, dims int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path = %s, want /embeddings", r.URL.Path)
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		data := make([]map[string]any, len(body.Input))
		for i := range body.Input {
			vec := make([]float64, dims)
			for j := range vec {
				vec[j] = float64(i) + float64(j)/10.0
			}
			// Return out of order to exercise index reassembly.
			data[len(body.Input)-1-i] = map[string]any{"index": i, "embedding": vec}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": body.Model,
			"data":  data,
			"usage": map[string]any{"prompt_tokens": 7, "total_tokens": 7},
		})
	}))
}

func TestEmbeddingConnector_SingleInput(t *testing.T) {
	server := embedTestServer(t, 3)
	defer server.Close()

	c := &EmbeddingConnector{Client: server.Client()}
	out, err := c.Execute(context.Background(), map[string]any{
		"model":       "text-embedding-3-small",
		"input":       "hello world",
		"base_url":    server.URL,
		"_credential": map[string]string{"api_key": "sk-test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if got := out["count"].(int); got != 1 {
		t.Errorf("count = %d, want 1", got)
	}
	if got := out["dimensions"].(int); got != 3 {
		t.Errorf("dimensions = %d, want 3", got)
	}
	// First input's vector is [0, 0.1, 0.2] → pgvector literal.
	if got := out["vector"].(string); got != "[0,0.1,0.2]" {
		t.Errorf("vector = %q, want %q", got, "[0,0.1,0.2]")
	}
	usage := out["usage"].(map[string]any)
	if usage["total_tokens"].(int) != 7 {
		t.Errorf("total_tokens = %v, want 7", usage["total_tokens"])
	}
}

func TestEmbeddingConnector_MultiInputPreservesOrder(t *testing.T) {
	server := embedTestServer(t, 2)
	defer server.Close()

	c := &EmbeddingConnector{Client: server.Client()}
	out, err := c.Execute(context.Background(), map[string]any{
		"model":    "text-embedding-3-small",
		"input":    []any{"a", "b", "c"},
		"base_url": server.URL,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out["count"].(int) != 3 {
		t.Fatalf("count = %d, want 3", out["count"])
	}
	vectors := out["vectors"].([]string)
	// Index reassembly: input i → vector [i, i+0.1].
	want := []string{"[0,0.1]", "[1,1.1]", "[2,2.1]"}
	for i, w := range want {
		if vectors[i] != w {
			t.Errorf("vectors[%d] = %q, want %q", i, vectors[i], w)
		}
	}
}

func TestEmbeddingConnector_Validation(t *testing.T) {
	c := &EmbeddingConnector{}
	if _, err := c.Execute(context.Background(), map[string]any{"input": "x"}); err == nil {
		t.Error("expected error when model is missing")
	}
	if _, err := c.Execute(context.Background(), map[string]any{"model": "m"}); err == nil {
		t.Error("expected error when input is missing")
	}
	if _, err := c.Execute(context.Background(), map[string]any{"model": "m", "input": ""}); err == nil {
		t.Error("expected error when input is empty")
	}
}

func TestEmbeddingConnector_ModelAllowlist(t *testing.T) {
	c := &EmbeddingConnector{AllowedModels: []string{"text-embedding-3-large"}}
	_, err := c.Execute(context.Background(), map[string]any{
		"model": "text-embedding-3-small",
		"input": "x",
	})
	if err == nil {
		t.Error("expected error for disallowed model")
	}
}

func TestEmbeddingConnector_IncompleteResponseFailsFast(t *testing.T) {
	// Server returns only one embedding for two inputs — must error rather
	// than silently returning a misaligned/short vectors list.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "text-embedding-3-small",
			"data":  []map[string]any{{"index": 0, "embedding": []float64{0.1, 0.2}}},
			"usage": map[string]any{"prompt_tokens": 3, "total_tokens": 3},
		})
	}))
	defer server.Close()

	c := &EmbeddingConnector{Client: server.Client()}
	_, err := c.Execute(context.Background(), map[string]any{
		"model":    "text-embedding-3-small",
		"input":    []any{"a", "b"},
		"base_url": server.URL,
	})
	if err == nil {
		t.Error("expected error when the provider returns fewer embeddings than inputs")
	}
}

func TestEmbeddingConnector_UnknownProvider(t *testing.T) {
	c := &EmbeddingConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"model":    "some-model",
		"input":    "x",
		"provider": "not-a-provider",
	})
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}
