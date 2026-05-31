package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEntraListUsersConnector_ListsUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer entra-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"value": []any{
				map[string]any{"id": "u1", "displayName": "Alice"},
				map[string]any{"id": "u2", "displayName": "Bob"},
			},
		})
	}))
	defer srv.Close()

	c := &EntraListUsersConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "entra-token"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
	users, ok := out["users"].([]any)
	if !ok || len(users) != 2 {
		t.Errorf("expected 2 users, got %v", out["users"])
	}
}

func TestEntraListUsersConnector_MissingCredential(t *testing.T) {
	c := &EntraListUsersConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestEntraCreateUserConnector_CreatesUser(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":                "user-123",
			"displayName":       "Alice Smith",
			"userPrincipalName": "alice@example.com",
		})
	}))
	defer srv.Close()

	c := &EntraCreateUserConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential":         map[string]string{"token": "entra-token"},
		"display_name":        "Alice Smith",
		"user_principal_name": "alice@example.com",
		"password":            "P@ssw0rd!",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotBody["displayName"] != "Alice Smith" {
		t.Errorf("displayName = %v, want Alice Smith", gotBody["displayName"])
	}
	if gotBody["userPrincipalName"] != "alice@example.com" {
		t.Errorf("userPrincipalName = %v", gotBody["userPrincipalName"])
	}
	// mailNickname should default to part before @
	if gotBody["mailNickname"] != "alice" {
		t.Errorf("mailNickname = %v, want alice", gotBody["mailNickname"])
	}
	if out["id"] != "user-123" {
		t.Errorf("id = %v, want user-123", out["id"])
	}
}

func TestEntraCreateUserConnector_MissingDisplayName(t *testing.T) {
	c := &EntraCreateUserConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential":         map[string]string{"token": "tok"},
		"user_principal_name": "alice@example.com",
		"password":            "P@ssw0rd!",
	})
	if err == nil {
		t.Fatal("expected error for missing display_name")
	}
}

func TestEntraAddGroupMemberConnector_AddsMember(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := &EntraAddGroupMemberConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "entra-token"},
		"group_id":    "group-abc",
		"user_id":     "user-123",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
	if gotBody["@odata.id"] != "https://graph.microsoft.com/v1.0/directoryObjects/user-123" {
		t.Errorf("@odata.id = %v", gotBody["@odata.id"])
	}
}

func TestEntraAddGroupMemberConnector_MissingGroupID(t *testing.T) {
	c := &EntraAddGroupMemberConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"user_id":     "user-123",
	})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

func TestRegistry_EntraConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"entra/list_users", "entra/create_user", "entra/add_group_member"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
