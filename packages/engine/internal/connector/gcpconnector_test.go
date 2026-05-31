package connector

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGCPPubSubPublishConnector_Publishes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer gcp-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		msgs, _ := body["messages"].([]any)
		if len(msgs) == 0 {
			t.Error("expected at least one message")
		} else {
			msg, _ := msgs[0].(map[string]any)
			data, _ := msg["data"].(string)
			decoded, _ := base64.StdEncoding.DecodeString(data)
			if string(decoded) != "hello pubsub" {
				t.Errorf("expected decoded data='hello pubsub', got %s", string(decoded))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"messageIds": []string{"1"},
		})
	}))
	defer srv.Close()

	c := &GCPPubSubPublishConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "gcp-token"},
		"project_id":  "my-project",
		"topic_id":    "my-topic",
		"message":     "hello pubsub",
	})
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := out["messageIds"].([]any)
	if len(ids) == 0 {
		t.Error("expected messageIds in response")
	}
}

func TestGCPPubSubPublishConnector_MissingProjectID(t *testing.T) {
	c := &GCPPubSubPublishConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"topic_id":    "my-topic",
		"message":     "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestGCPPubSubPublishConnector_MissingMessage(t *testing.T) {
	c := &GCPPubSubPublishConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"project_id":  "my-project",
		"topic_id":    "my-topic",
	})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestGCPPubSubPublishConnector_MissingCredential(t *testing.T) {
	c := &GCPPubSubPublishConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"project_id": "my-project",
		"topic_id":   "my-topic",
		"message":    "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestGCPInvokeCloudRunConnector_Invokes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Bearer auth, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"result": "ok",
		})
	}))
	defer srv.Close()

	c := &GCPInvokeCloudRunConnector{}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "run-token"},
		"url":         srv.URL + "/run",
		"body":        `{"input":"data"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["result"] != "ok" {
		t.Errorf("expected result=ok, got %v", out["result"])
	}
}

func TestGCPInvokeCloudRunConnector_MissingURL(t *testing.T) {
	c := &GCPInvokeCloudRunConnector{}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestRegistry_GCPConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"gcp/publish", "gcp/invoke_cloud_run"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
