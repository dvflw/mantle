package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMailchimpListMembersConnector_ListsMembers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/3.0/lists/abc123/members" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, pass, ok := r.BasicAuth()
		if !ok || pass != "testkey-us1" {
			t.Errorf("unexpected basic auth password: %s", pass)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"members":     []any{map[string]any{"id": "m1", "email_address": "alice@example.com"}},
			"total_items": 1,
		})
	}))
	defer srv.Close()

	// Override baseURL to use the test server; DC is "us1" from the key.
	c := &MailchimpListMembersConnector{baseURL: srv.URL + "/3.0"}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "testkey-us1"},
		"list_id":     "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 1 {
		t.Errorf("expected count=1, got %v", out["count"])
	}
	if out["total_items"].(int) != 1 {
		t.Errorf("expected total_items=1, got %v", out["total_items"])
	}
}

func TestMailchimpListMembersConnector_MissingListID(t *testing.T) {
	c := &MailchimpListMembersConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key-us1"},
	})
	if err == nil {
		t.Fatal("expected error for missing list_id")
	}
}

func TestMailchimpListMembersConnector_MissingCredential(t *testing.T) {
	c := &MailchimpListMembersConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"list_id": "abc",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestMailchimpListMembersConnector_InvalidAPIKeyFormat(t *testing.T) {
	c := &MailchimpListMembersConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "nokeydc"},
		"list_id":     "abc",
	})
	if err == nil {
		t.Fatal("expected error for invalid api_key format")
	}
}

func TestMailchimpAddMemberConnector_AddsMember(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/3.0/lists/list1/members" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["email_address"] != "bob@example.com" {
			t.Errorf("unexpected email: %v", body["email_address"])
		}
		if body["status"] != "subscribed" {
			t.Errorf("unexpected status: %v", body["status"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"id": "m2", "email_address": "bob@example.com", "status": "subscribed"})
	}))
	defer srv.Close()

	c := &MailchimpAddMemberConnector{baseURL: srv.URL + "/3.0"}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key-us1"},
		"list_id":     "list1",
		"email":       "bob@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "m2" {
		t.Errorf("expected id=m2, got %v", out["id"])
	}
}

func TestMailchimpAddMemberConnector_MissingEmail(t *testing.T) {
	c := &MailchimpAddMemberConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key-us1"},
		"list_id":     "abc",
	})
	if err == nil {
		t.Fatal("expected error for missing email")
	}
}

func TestMailchimpAddMemberConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"title":"Invalid Resource","status":400}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c := &MailchimpAddMemberConnector{baseURL: srv.URL + "/3.0"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key-us1"},
		"list_id":     "bad",
		"email":       "x@y.com",
	})
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

func TestMailchimpListMembersConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, _ := r.BasicAuth()
		if pass != "mapkey-us2" {
			t.Errorf("unexpected auth: %s", pass)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"members": []any{}, "total_items": 0})
	}))
	defer srv.Close()

	c := &MailchimpListMembersConnector{baseURL: srv.URL + "/3.0"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"api_key": "mapkey-us2"},
		"list_id":     "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_MailchimpConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"mailchimp/list_members", "mailchimp/add_member"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
