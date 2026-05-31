package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQuickBooksCreateInvoiceConnector_CreatesInvoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer qb-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		lineItems, _ := body["Line"].([]any)
		if len(lineItems) != 1 {
			t.Errorf("expected 1 line item, got %d", len(lineItems))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"Invoice": map[string]any{"Id": "1", "DocNumber": "INV-001"},
		})
	}))
	defer srv.Close()

	c := &QuickBooksCreateInvoiceConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"realm_id": "123456", "access_token": "qb-token"},
		"line": []any{
			map[string]any{
				"Amount":           100.0,
				"DetailType":       "SalesItemLineDetail",
				"SalesItemLineDetail": map[string]any{"ItemRef": map[string]any{"value": "1", "name": "Services"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["Invoice"] == nil {
		t.Errorf("expected Invoice in response, got %v", out)
	}
}

func TestQuickBooksCreateInvoiceConnector_MissingLine(t *testing.T) {
	c := &QuickBooksCreateInvoiceConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"realm_id": "123", "access_token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing line")
	}
}

func TestQuickBooksCreateInvoiceConnector_MissingCredential(t *testing.T) {
	c := &QuickBooksCreateInvoiceConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"line": []any{map[string]any{"Amount": 100}},
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestQuickBooksCreateInvoiceConnector_MissingRealmID(t *testing.T) {
	c := &QuickBooksCreateInvoiceConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"access_token": "tok"},
		"line":        []any{map[string]any{"Amount": 100}},
	})
	if err == nil {
		t.Fatal("expected error for missing realm_id")
	}
}

func TestQuickBooksListInvoicesConnector_ListsInvoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"QueryResponse": map[string]any{
				"Invoice":    []any{map[string]any{"Id": "1"}, map[string]any{"Id": "2"}},
				"totalCount": 2,
			},
		})
	}))
	defer srv.Close()

	c := &QuickBooksListInvoicesConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"realm_id": "123", "access_token": "tok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestQuickBooksListInvoicesConnector_MissingAccessToken(t *testing.T) {
	c := &QuickBooksListInvoicesConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"realm_id": "123"},
	})
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
}

func TestQuickBooksCreateInvoiceConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"Fault":{"Error":[{"Message":"Token expired"}]}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &QuickBooksCreateInvoiceConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"realm_id": "123", "access_token": "bad"},
		"line":        []any{map[string]any{"Amount": 100}},
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestQuickBooksCreateInvoiceConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer maptoken" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"Invoice": map[string]any{"Id": "2"}})
	}))
	defer srv.Close()

	c := &QuickBooksCreateInvoiceConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"realm_id": "123", "access_token": "maptoken"},
		"line":        []any{map[string]any{"Amount": 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_QuickBooksConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"quickbooks/create_invoice", "quickbooks/list_invoices"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
