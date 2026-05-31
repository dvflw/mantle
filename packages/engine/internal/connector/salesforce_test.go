package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSalesforceQueryConnector_QueriesRecords(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer sf-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"records":   []any{map[string]any{"Id": "001"}, map[string]any{"Id": "002"}},
			"totalSize": 2,
		})
	}))
	defer srv.Close()

	c := &SalesforceQueryConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"access_token": "sf-token",
			"instance_url": "https://myorg.salesforce.com",
		},
		"soql": "SELECT Id FROM Account",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotPath != "/query" {
		t.Errorf("path = %q, want /query", gotPath)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
	if out["total_size"].(int) != 2 {
		t.Errorf("expected total_size=2, got %v", out["total_size"])
	}
}

func TestSalesforceQueryConnector_SOQLEncoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		decoded, _ := url.QueryUnescape(q)
		if !strings.Contains(decoded, "SELECT") {
			t.Errorf("expected SOQL in query param, got %q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}, "totalSize": 0})
	}))
	defer srv.Close()

	c := &SalesforceQueryConnector{baseURL: srv.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"access_token": "tok",
			"instance_url": "https://myorg.salesforce.com",
		},
		"soql": "SELECT Id, Name FROM Contact WHERE Email = 'alice@example.com'",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

func TestSalesforceQueryConnector_MissingSOQL(t *testing.T) {
	c := &SalesforceQueryConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"access_token": "tok",
			"instance_url": "https://myorg.salesforce.com",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing soql")
	}
}

func TestSalesforceQueryConnector_MissingCredential(t *testing.T) {
	c := &SalesforceQueryConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"soql": "SELECT Id FROM Account",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestSalesforceQueryConnector_MissingInstanceURL(t *testing.T) {
	c := &SalesforceQueryConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"access_token": "tok"},
		"soql":        "SELECT Id FROM Account",
	})
	if err == nil {
		t.Fatal("expected error for missing instance_url")
	}
}

func TestSalesforceCreateRecordConnector_CreatesRecord(t *testing.T) {
	var gotBody map[string]any
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "001xx000003GYn1",
			"success": true,
		})
	}))
	defer srv.Close()

	c := &SalesforceCreateRecordConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"access_token": "sf-token",
			"instance_url": "https://myorg.salesforce.com",
		},
		"object_type": "Account",
		"fields": map[string]any{
			"Name":    "Acme Corp",
			"Website": "https://acme.com",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotPath != "/sobjects/Account" {
		t.Errorf("path = %q, want /sobjects/Account", gotPath)
	}
	if gotBody["Name"] != "Acme Corp" {
		t.Errorf("Name = %v", gotBody["Name"])
	}
	if out["id"] != "001xx000003GYn1" {
		t.Errorf("id = %v", out["id"])
	}
}

func TestSalesforceCreateRecordConnector_MissingObjectType(t *testing.T) {
	c := &SalesforceCreateRecordConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"access_token": "tok",
			"instance_url": "https://myorg.salesforce.com",
		},
		"fields": map[string]any{"Name": "Acme"},
	})
	if err == nil {
		t.Fatal("expected error for missing object_type")
	}
}

func TestSalesforceCreateRecordConnector_MissingFields(t *testing.T) {
	c := &SalesforceCreateRecordConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"access_token": "tok",
			"instance_url": "https://myorg.salesforce.com",
		},
		"object_type": "Account",
	})
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestRegistry_SalesforceConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"salesforce/query", "salesforce/create_record"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
