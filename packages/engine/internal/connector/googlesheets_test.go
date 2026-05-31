package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleSheetsReadRangeConnector_ReadsRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer sheets-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"values": []any{
				[]any{"A1", "B1"},
				[]any{"A2", "B2"},
			},
		})
	}))
	defer srv.Close()

	c := &GoogleSheetsReadRangeConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential":    map[string]string{"token": "sheets-token"},
		"spreadsheet_id": "spreadsheet-1",
		"range":          "Sheet1!A1:B2",
	})
	if err != nil {
		t.Fatal(err)
	}
	values, _ := out["values"].([][]any)
	if len(values) != 2 {
		t.Errorf("expected 2 rows, got %d", len(values))
	}
	if out["row_count"].(int) != 2 {
		t.Errorf("expected row_count=2, got %v", out["row_count"])
	}
}

func TestGoogleSheetsReadRangeConnector_MissingSpreadsheetID(t *testing.T) {
	c := &GoogleSheetsReadRangeConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"range":       "Sheet1!A1:B2",
	})
	if err == nil {
		t.Fatal("expected error for missing spreadsheet_id")
	}
}

func TestGoogleSheetsReadRangeConnector_MissingRange(t *testing.T) {
	c := &GoogleSheetsReadRangeConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential":    map[string]string{"token": "tok"},
		"spreadsheet_id": "spreadsheet-1",
	})
	if err == nil {
		t.Fatal("expected error for missing range")
	}
}

func TestGoogleSheetsReadRangeConnector_MissingCredential(t *testing.T) {
	c := &GoogleSheetsReadRangeConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"spreadsheet_id": "spreadsheet-1",
		"range":          "Sheet1!A1:B2",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestGoogleSheetsAppendRowsConnector_AppendsRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer append-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"spreadsheetId": "spreadsheet-1",
			"tableRange":    "Sheet1!A1:B2",
			"updates": map[string]any{
				"updatedRows": 2,
			},
		})
	}))
	defer srv.Close()

	c := &GoogleSheetsAppendRowsConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential":    map[string]string{"token": "append-token"},
		"spreadsheet_id": "spreadsheet-1",
		"range":          "Sheet1!A1",
		"values": []any{
			[]any{"val1", "val2"},
			[]any{"val3", "val4"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["spreadsheetId"] != "spreadsheet-1" {
		t.Errorf("expected spreadsheetId=spreadsheet-1, got %v", out["spreadsheetId"])
	}
}

func TestGoogleSheetsAppendRowsConnector_MissingValues(t *testing.T) {
	c := &GoogleSheetsAppendRowsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential":    map[string]string{"token": "tok"},
		"spreadsheet_id": "spreadsheet-1",
		"range":          "Sheet1!A1",
	})
	if err == nil {
		t.Fatal("expected error for missing values")
	}
}

func TestRegistry_GoogleSheetsConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"sheets/read_range", "sheets/append_rows"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
