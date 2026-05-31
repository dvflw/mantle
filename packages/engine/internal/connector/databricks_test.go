package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatabricksExecuteSQLConnector_ExecutesSQL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/2.0/sql/statements" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer dapi-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		if body["warehouse_id"] != "wh-123" {
			t.Errorf("expected warehouse_id=wh-123, got %v", body["warehouse_id"])
		}
		if body["statement"] != "SELECT 1" {
			t.Errorf("expected statement='SELECT 1', got %v", body["statement"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"statement_id": "stmt-1",
			"status":       map[string]any{"state": "SUCCEEDED"},
			"result":       map[string]any{"data_array": []any{}},
		})
	}))
	defer srv.Close()

	c := &DatabricksExecuteSQLConnector{}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":  srv.URL,
			"token": "dapi-token",
		},
		"warehouse_id": "wh-123",
		"statement":    "SELECT 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["statement_id"] != "stmt-1" {
		t.Errorf("expected statement_id=stmt-1, got %v", out["statement_id"])
	}
}

func TestDatabricksExecuteSQLConnector_MissingWarehouseID(t *testing.T) {
	c := &DatabricksExecuteSQLConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":  "https://adb-1234.azuredatabricks.net",
			"token": "dapi-token",
		},
		"statement": "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing warehouse_id")
	}
}

func TestDatabricksExecuteSQLConnector_MissingStatement(t *testing.T) {
	c := &DatabricksExecuteSQLConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":  "https://adb-1234.azuredatabricks.net",
			"token": "dapi-token",
		},
		"warehouse_id": "wh-123",
	})
	if err == nil {
		t.Fatal("expected error for missing statement")
	}
}

func TestDatabricksExecuteSQLConnector_MissingCredential(t *testing.T) {
	c := &DatabricksExecuteSQLConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"warehouse_id": "wh-123",
		"statement":    "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestDatabricksExecuteSQLConnector_MissingHost(t *testing.T) {
	c := &DatabricksExecuteSQLConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"token": "dapi-token",
		},
		"warehouse_id": "wh-123",
		"statement":    "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestDatabricksSubmitJobConnector_SubmitsJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/2.1/jobs/runs/submit" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer dapi-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"run_id": float64(42),
		})
	}))
	defer srv.Close()

	c := &DatabricksSubmitJobConnector{}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":  srv.URL,
			"token": "dapi-token",
		},
		"run_name": "my-run",
		"tasks": []any{
			map[string]any{
				"task_key":          "task1",
				"existing_cluster_id": "cluster-123",
				"notebook_task": map[string]any{
					"notebook_path": "/Users/me/notebook",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runID, ok := extractInt(out["run_id"])
	if !ok || runID != 42 {
		t.Errorf("expected run_id=42, got %v", out["run_id"])
	}
}

func TestDatabricksSubmitJobConnector_MissingTasks(t *testing.T) {
	c := &DatabricksSubmitJobConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{
			"host":  "https://adb-1234.azuredatabricks.net",
			"token": "dapi-token",
		},
		"run_name": "my-run",
	})
	if err == nil {
		t.Fatal("expected error for missing tasks")
	}
}

func TestDatabricksSubmitJobConnector_MissingCredential(t *testing.T) {
	c := &DatabricksSubmitJobConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"tasks": []any{},
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRegistry_DatabricksConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"databricks/execute_sql", "databricks/submit_job"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
