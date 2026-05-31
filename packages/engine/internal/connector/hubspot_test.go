package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHubSpotCreateContactConnector_CreatesContact(t *testing.T) {
	var gotBody map[string]any
	var gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":         "contact-1",
			"properties": map[string]any{"email": "alice@example.com"},
		})
	}))
	defer srv.Close()

	c := &HubSpotCreateContactConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "hs-token"},
		"email":       "alice@example.com",
		"firstname":   "Alice",
		"lastname":    "Smith",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	props, _ := gotBody["properties"].(map[string]any)
	if props["email"] != "alice@example.com" {
		t.Errorf("email = %v", props["email"])
	}
	if out["id"] != "contact-1" {
		t.Errorf("id = %v, want contact-1", out["id"])
	}
}

func TestHubSpotCreateContactConnector_MissingEmail(t *testing.T) {
	c := &HubSpotCreateContactConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing email")
	}
}

func TestHubSpotCreateContactConnector_MissingCredential(t *testing.T) {
	c := &HubSpotCreateContactConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"email": "alice@example.com",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestHubSpotSearchContactsConnector_SearchesContacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []any{
				map[string]any{"id": "c1", "properties": map[string]any{"email": "alice@example.com"}},
				map[string]any{"id": "c2", "properties": map[string]any{"email": "bob@example.com"}},
			},
		})
	}))
	defer srv.Close()

	c := &HubSpotSearchContactsConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "hs-token"},
		"query":       "alice",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
	results, ok := out["results"].([]any)
	if !ok || len(results) != 2 {
		t.Errorf("expected 2 results, got %v", out["results"])
	}
}

func TestHubSpotSearchContactsConnector_MissingQuery(t *testing.T) {
	c := &HubSpotSearchContactsConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestRegistry_HubSpotConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"hubspot/create_contact", "hubspot/search_contacts"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
