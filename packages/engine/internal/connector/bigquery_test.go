package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func bqTestCredential(projectID string) map[string]string {
	return map[string]string{
		"project_id": projectID,
		"token":      "bq-token",
	}
}

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
			"jobComplete": true,
			"jobReference": map[string]any{
				"projectId": "my-project",
				"jobId":     "job-1",
			},
			"schema": map[string]any{
				"fields": []any{
					map[string]any{"name": "col1", "type": "STRING", "mode": "NULLABLE"},
				},
			},
			"rows": []any{
				map[string]any{"f": []any{map[string]any{"v": "value1"}}},
				map[string]any{"f": []any{map[string]any{"v": "value2"}}},
			},
			"totalRows": "2",
		})
	}))
	defer srv.Close()

	c := &BigQueryQueryConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("my-project"),
		"query":       "SELECT 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["rows"].([]map[string]any)
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["col1"] != "value1" {
		t.Errorf("expected rows[0][col1]=value1, got %v", rows[0]["col1"])
	}
	if out["schema"] == nil {
		t.Error("expected schema in response")
	}
	if out["total_rows"].(int) != 2 {
		t.Errorf("expected total_rows=2, got %v", out["total_rows"])
	}
}

func TestBigQueryQueryConnector_NullValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jobComplete": true,
			"jobReference": map[string]any{"projectId": "p", "jobId": "j"},
			"schema": map[string]any{
				"fields": []any{
					map[string]any{"name": "a", "type": "STRING", "mode": "NULLABLE"},
					map[string]any{"name": "b", "type": "INTEGER", "mode": "NULLABLE"},
				},
			},
			"rows": []any{
				map[string]any{"f": []any{map[string]any{"v": "hello"}, map[string]any{"v": nil}}},
			},
			"totalRows": "1",
		})
	}))
	defer srv.Close()

	c := &BigQueryQueryConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("p"),
		"query":       "SELECT a, b FROM t",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["rows"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["a"] != "hello" {
		t.Errorf("expected a=hello, got %v", rows[0]["a"])
	}
	if rows[0]["b"] != nil {
		t.Errorf("expected b=nil (NULL), got %v", rows[0]["b"])
	}
}

func TestBigQueryQueryConnector_RepeatedField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jobComplete": true,
			"jobReference": map[string]any{"projectId": "p", "jobId": "j"},
			"schema": map[string]any{
				"fields": []any{
					map[string]any{"name": "tags", "type": "STRING", "mode": "REPEATED"},
				},
			},
			"rows": []any{
				map[string]any{
					"f": []any{
						map[string]any{
							"v": []any{
								map[string]any{"v": "go"},
								map[string]any{"v": "sql"},
							},
						},
					},
				},
			},
			"totalRows": "1",
		})
	}))
	defer srv.Close()

	c := &BigQueryQueryConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("p"),
		"query":       "SELECT tags FROM t",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["rows"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	tags, _ := rows[0]["tags"].([]any)
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %v", tags)
	}
	if tags[0] != "go" || tags[1] != "sql" {
		t.Errorf("unexpected tag values: %v", tags)
	}
}

func TestBigQueryQueryConnector_RecordType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"jobComplete": true,
			"jobReference": map[string]any{"projectId": "p", "jobId": "j"},
			"schema": map[string]any{
				"fields": []any{
					map[string]any{
						"name": "addr", "type": "RECORD", "mode": "NULLABLE",
						"fields": []any{
							map[string]any{"name": "city", "type": "STRING", "mode": "NULLABLE"},
							map[string]any{"name": "zip", "type": "STRING", "mode": "NULLABLE"},
						},
					},
				},
			},
			"rows": []any{
				map[string]any{
					"f": []any{
						map[string]any{
							"v": map[string]any{
								"f": []any{
									map[string]any{"v": "Portland"},
									map[string]any{"v": "97201"},
								},
							},
						},
					},
				},
			},
			"totalRows": "1",
		})
	}))
	defer srv.Close()

	c := &BigQueryQueryConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("p"),
		"query":       "SELECT addr FROM t",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["rows"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	addr, _ := rows[0]["addr"].(map[string]any)
	if addr["city"] != "Portland" || addr["zip"] != "97201" {
		t.Errorf("unexpected addr: %v", addr)
	}
}

func TestBigQueryQueryConnector_Pagination(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := callCount.Add(1)

		fields := []any{
			map[string]any{"name": "val", "type": "STRING", "mode": "NULLABLE"},
		}

		if r.Method == "POST" {
			// Initial query — return first page with pageToken
			json.NewEncoder(w).Encode(map[string]any{
				"jobComplete": true,
				"jobReference": map[string]any{"projectId": "p", "jobId": "job-1"},
				"schema":       map[string]any{"fields": fields},
				"rows": []any{
					map[string]any{"f": []any{map[string]any{"v": "row1"}}},
				},
				"totalRows": "2",
				"pageToken": "page2",
			})
			return
		}

		// GET getQueryResults — second page
		if pageToken := r.URL.Query().Get("pageToken"); pageToken != "page2" {
			t.Errorf("call %d: unexpected pageToken %q", n, pageToken)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"jobComplete": true,
			"jobReference": map[string]any{"projectId": "p", "jobId": "job-1"},
			"schema":       map[string]any{"fields": fields},
			"rows": []any{
				map[string]any{"f": []any{map[string]any{"v": "row2"}}},
			},
			"totalRows": "2",
			"pageToken": "",
		})
	}))
	defer srv.Close()

	c := &BigQueryQueryConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("p"),
		"query":       "SELECT val FROM t",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["rows"].([]map[string]any)
	if len(rows) != 2 {
		t.Errorf("expected 2 rows after pagination, got %d", len(rows))
	}
	if rows[0]["val"] != "row1" || rows[1]["val"] != "row2" {
		t.Errorf("unexpected row values: %v", rows)
	}
}

func TestBigQueryQueryConnector_JobNotComplete(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := callCount.Add(1)

		fields := []any{
			map[string]any{"name": "x", "type": "STRING", "mode": "NULLABLE"},
		}

		if r.Method == "POST" {
			// Initial query — job not done yet
			json.NewEncoder(w).Encode(map[string]any{
				"jobComplete":  false,
				"jobReference": map[string]any{"projectId": "p", "jobId": "job-async"},
			})
			return
		}

		// Polling calls
		if n < 3 {
			// Still not done on second call
			json.NewEncoder(w).Encode(map[string]any{
				"jobComplete":  false,
				"jobReference": map[string]any{"projectId": "p", "jobId": "job-async"},
			})
			return
		}

		// Done on third call
		json.NewEncoder(w).Encode(map[string]any{
			"jobComplete": true,
			"jobReference": map[string]any{"projectId": "p", "jobId": "job-async"},
			"schema":       map[string]any{"fields": fields},
			"rows": []any{
				map[string]any{"f": []any{map[string]any{"v": "done"}}},
			},
			"totalRows": "1",
		})
	}))
	defer srv.Close()

	c := &BigQueryQueryConnector{
		baseURL:      srv.URL,
		pollInterval: time.Millisecond,
	}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("p"),
		"query":       "SELECT x FROM slow_table",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["rows"].([]map[string]any)
	if len(rows) != 1 || rows[0]["x"] != "done" {
		t.Errorf("unexpected rows: %v", rows)
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
		"_credential": bqTestCredential("my-project"),
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
		"_credential": bqTestCredential("my-project"),
		"dataset_id":  "my_dataset",
		"table_id":    "my_table",
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
		"_credential": bqTestCredential("my-project"),
		"table_id":    "my_table",
		"rows":        []any{},
	})
	if err == nil {
		t.Fatal("expected error for missing dataset_id")
	}
}

func TestBigQueryInsertRowsConnector_MissingTableID(t *testing.T) {
	c := &BigQueryInsertRowsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("my-project"),
		"dataset_id":  "my_dataset",
		"rows":        []any{},
	})
	if err == nil {
		t.Fatal("expected error for missing table_id")
	}
}

func TestBigQueryInsertRowsConnector_MissingRows(t *testing.T) {
	c := &BigQueryInsertRowsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": bqTestCredential("my-project"),
		"dataset_id":  "my_dataset",
		"table_id":    "my_table",
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
