package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOktaListUsersConnector_ListsUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/users" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "SSWS ssws-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{
			map[string]any{"id": "00u1", "profile": map[string]any{"login": "alice@example.com"}},
			map[string]any{"id": "00u2", "profile": map[string]any{"login": "bob@example.com"}},
		})
	}))
	defer srv.Close()

	c := &OktaListUsersConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"domain": "dev-xxx.okta.com", "token": "ssws-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestOktaListUsersConnector_WithQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "alice" {
			t.Errorf("expected q=alice, got %s", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	c := &OktaListUsersConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"domain": "dev.okta.com", "token": "tok"},
		"q":           "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOktaListUsersConnector_MissingCredential(t *testing.T) {
	c := &OktaListUsersConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestOktaListUsersConnector_MissingDomain(t *testing.T) {
	c := &OktaListUsersConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing domain")
	}
}

func TestOktaCreateUserConnector_CreatesUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("activate") != "true" {
			t.Errorf("expected activate=true, got %q", r.URL.Query().Get("activate"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		profile, _ := body["profile"].(map[string]any)
		if profile["login"] != "alice@example.com" {
			t.Errorf("unexpected login: %v", profile["login"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"id": "00u1", "status": "ACTIVE"})
	}))
	defer srv.Close()

	c := &OktaCreateUserConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"domain": "dev.okta.com", "token": "tok"},
		"profile": map[string]any{
			"login":     "alice@example.com",
			"email":     "alice@example.com",
			"firstName": "Alice",
			"lastName":  "Smith",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "00u1" {
		t.Errorf("expected id=00u1, got %v", out["id"])
	}
}

func TestOktaCreateUserConnector_ActivateFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("activate") != "false" {
			t.Errorf("expected activate=false, got %q", r.URL.Query().Get("activate"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"id": "00u2", "status": "STAGED"})
	}))
	defer srv.Close()

	c := &OktaCreateUserConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"domain": "dev.okta.com", "token": "tok"},
		"activate":    false,
		"profile":     map[string]any{"login": "staged@example.com", "email": "staged@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "00u2" {
		t.Errorf("expected id=00u2, got %v", out["id"])
	}
}

func TestOktaCreateUserConnector_MissingProfile(t *testing.T) {
	c := &OktaCreateUserConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"domain": "dev.okta.com", "token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestOktaCreateUserConnector_MissingLogin(t *testing.T) {
	c := &OktaCreateUserConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"domain": "dev.okta.com", "token": "tok"},
		"profile":     map[string]any{"email": "alice@example.com"},
	})
	if err == nil {
		t.Fatal("expected error for missing profile.login")
	}
}

func TestOktaListUsersConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errorCode":"E0000011","errorSummary":"Invalid token"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &OktaListUsersConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"domain": "dev.okta.com", "token": "bad"},
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestOktaListUsersConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "SSWS maptoken" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	c := &OktaListUsersConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"domain": "dev.okta.com", "token": "maptoken"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_OktaConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"okta/list_users", "okta/create_user"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
