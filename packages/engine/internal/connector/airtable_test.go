package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAirtableListConnector_ListsRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer pat123" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"records": []any{
				map[string]any{"id": "rec1", "fields": map[string]any{"Name": "Task 1"}},
				map[string]any{"id": "rec2", "fields": map[string]any{"Name": "Task 2"}},
			},
		})
	}))
	defer srv.Close()

	c := &AirtableListConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"base_id":     "appXXX",
		"table_id":    "tblYYY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestAirtableListConnector_WithQueryParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("maxRecords") != "10" {
			t.Errorf("expected maxRecords=10, got %s", q.Get("maxRecords"))
		}
		if q.Get("filterByFormula") != "NOT({Done})" {
			t.Errorf("unexpected filterByFormula: %s", q.Get("filterByFormula"))
		}
		if q.Get("view") != "Grid" {
			t.Errorf("unexpected view: %s", q.Get("view"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	}))
	defer srv.Close()

	c := &AirtableListConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential":       map[string]string{"token": "pat123"},
		"base_id":           "appXXX",
		"table_id":          "tblYYY",
		"max_records":       10,
		"filter_by_formula": "NOT({Done})",
		"view":              "Grid",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAirtableListConnector_HasMore_ReturnsOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"records": []any{map[string]any{"id": "rec1"}},
			"offset":  "itrABC",
		})
	}))
	defer srv.Close()

	c := &AirtableListConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"base_id":     "appXXX",
		"table_id":    "tblYYY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["offset"] != "itrABC" {
		t.Errorf("expected offset=itrABC, got %v", out["offset"])
	}
}

func TestAirtableListConnector_MissingBaseID(t *testing.T) {
	c := &AirtableListConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"table_id":    "tblYYY",
	})
	if err == nil {
		t.Fatal("expected error for missing base_id")
	}
}

func TestAirtableListConnector_MissingCredential(t *testing.T) {
	c := &AirtableListConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"base_id":  "appXXX",
		"table_id": "tblYYY",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestAirtableListConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"NOT_FOUND"}}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := &AirtableListConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"base_id":     "appXXX",
		"table_id":    "tblYYY",
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestAirtableCreateRecordConnector_CreatesRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		fields, _ := body["fields"].(map[string]any)
		if fields["Name"] != "My Record" {
			t.Errorf("unexpected fields: %v", fields)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "recABC",
			"createdTime": "2024-01-01T00:00:00.000Z",
			"fields":      map[string]any{"Name": "My Record"},
		})
	}))
	defer srv.Close()

	c := &AirtableCreateRecordConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"base_id":     "appXXX",
		"table_id":    "tblYYY",
		"fields":      map[string]any{"Name": "My Record"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "recABC" {
		t.Errorf("expected id=recABC, got %v", out["id"])
	}
}

func TestAirtableCreateRecordConnector_EmptyFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		fields, _ := body["fields"].(map[string]any)
		if fields == nil {
			t.Error("expected empty fields map, got nil")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "recDEF",
			"createdTime": "2024-01-01T00:00:00.000Z",
			"fields":      map[string]any{},
		})
	}))
	defer srv.Close()

	c := &AirtableCreateRecordConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"base_id":     "appXXX",
		"table_id":    "tblYYY",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAirtableCreateRecordConnector_MissingTableID(t *testing.T) {
	c := &AirtableCreateRecordConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"base_id":     "appXXX",
	})
	if err == nil {
		t.Fatal("expected error for missing table_id")
	}
}

func TestAirtableCreateRecordConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"type":"INVALID_REQUEST_BODY"}}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	c := &AirtableCreateRecordConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "pat123"},
		"base_id":     "appXXX",
		"table_id":    "tblYYY",
	})
	if err == nil {
		t.Fatal("expected error for 422")
	}
}

func TestAirtableListConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mapany" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	}))
	defer srv.Close()

	c := &AirtableListConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"token": "mapany"},
		"base_id":     "appXXX",
		"table_id":    "tblYYY",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_AirtableConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("airtable/list"); err != nil {
		t.Errorf("airtable/list not registered: %v", err)
	}
	if _, err := r.Get("airtable/create_record"); err != nil {
		t.Errorf("airtable/create_record not registered: %v", err)
	}
}
