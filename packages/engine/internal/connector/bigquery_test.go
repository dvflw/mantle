package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBigQueryQueryConnector_ExecutesQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer bq-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		if body["query"] != "SELECT 1" {
			t.Errorf("expected query='SELECT 1', got %v", body["query"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"schema": map[string]any{
				"fields": []any{
					map[string]any{"name": "col1", "type": "STRING"},
				},
			},
			"rows": []any{
				map[string]any{
					"f": []any{
						map[string]any{"v": "value1"},
					},
				},
				map[string]any{
					"f": []any{
						map[string]any{"v": "value2"},
					},
				},
			},
			"totalRows": "2",
		})
	}))
	defer srv.Close()

	c := &BigQueryQueryConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"project_id": "my-project",
			"token":      "bq-token",
		},
		"query": "SELECT 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["rows"].([][]string)
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
	if out["schema"] == nil {
		t.Error("expected schema in response")
	}
	if out["total_rows"].(int) != 2 {
		t.Errorf("expected total_rows=2, got %v", out["total_rows"])
	}
}

func TestBigQueryQueryConnector_MissingCredential(t *testing.T) {
	c := &BigQueryQueryConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestBigQueryQueryConnector_MissingProjectID(t *testing.T) {
	c := &BigQueryQueryConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"token": "bq-token",
		},
		"query": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestBigQueryQueryConnector_MissingQuery(t *testing.T) {
	c := &BigQueryQueryConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"project_id": "my-project",
			"token":      "bq-token",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestBigQueryInsertRowsConnector_InsertsRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer bq-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		rows, _ := body["rows"].([]any)
		if len(rows) != 2 {
			t.Errorf("expected 2 rows in request, got %d", len(rows))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"kind":         "bigquery#tableDataInsertAllResponse",
			"insertErrors": nil,
		})
	}))
	defer srv.Close()

	c := &BigQueryInsertRowsConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"project_id": "my-project",
			"token":      "bq-token",
		},
		"dataset_id": "my_dataset",
		"table_id":   "my_table",
		"rows": []any{
			map[string]any{"col1": "val1", "col2": "val2"},
			map[string]any{"col1": "val3", "col2": "val4"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["kind"] != "bigquery#tableDataInsertAllResponse" {
		t.Errorf("unexpected response kind: %v", out["kind"])
	}
}

func TestBigQueryInsertRowsConnector_MissingDatasetID(t *testing.T) {
	c := &BigQueryInsertRowsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"project_id": "my-project",
			"token":      "bq-token",
		},
		"table_id": "my_table",
		"rows":     []any{},
	})
	if err == nil {
		t.Fatal("expected error for missing dataset_id")
	}
}

func TestBigQueryInsertRowsConnector_MissingTableID(t *testing.T) {
	c := &BigQueryInsertRowsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"project_id": "my-project",
			"token":      "bq-token",
		},
		"dataset_id": "my_dataset",
		"rows":       []any{},
	})
	if err == nil {
		t.Fatal("expected error for missing table_id")
	}
}

func TestBigQueryInsertRowsConnector_MissingRows(t *testing.T) {
	c := &BigQueryInsertRowsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"project_id": "my-project",
			"token":      "bq-token",
		},
		"dataset_id": "my_dataset",
		"table_id":   "my_table",
	})
	if err == nil {
		t.Fatal("expected error for missing rows")
	}
}

func TestBigQueryInsertRowsConnector_MissingCredential(t *testing.T) {
	c := &BigQueryInsertRowsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"dataset_id": "my_dataset",
		"table_id":   "my_table",
		"rows":       []any{},
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRegistry_BigQueryConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"bigquery/query", "bigquery/insert_rows"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
