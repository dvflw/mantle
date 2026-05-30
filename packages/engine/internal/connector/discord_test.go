package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscordSendConnector_SendsMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bot bot-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["content"] != "Hello, world!" {
			t.Errorf("unexpected content: %v", body["content"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         "1234567890",
			"channel_id": "9876543210",
			"timestamp":  "2024-01-01T00:00:00.000Z",
		})
	}))
	defer srv.Close()

	c := &DiscordSendConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bot-token"},
		"channel_id":  "9876543210",
		"content":     "Hello, world!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "1234567890" {
		t.Errorf("expected id=1234567890, got %v", out["id"])
	}
}

func TestDiscordSendConnector_MissingContent(t *testing.T) {
	c := &DiscordSendConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bot-token"},
		"channel_id":  "9876543210",
	})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestDiscordSendConnector_MissingChannelID(t *testing.T) {
	c := &DiscordSendConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bot-token"},
		"content":     "Hello",
	})
	if err == nil {
		t.Fatal("expected error for missing channel_id")
	}
}

func TestDiscordSendConnector_MissingCredential(t *testing.T) {
	c := &DiscordSendConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"channel_id": "9876543210",
		"content":    "Hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestDiscordSendConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":50013,"message":"Missing Permissions"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &DiscordSendConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bot-token"},
		"channel_id":  "9876543210",
		"content":     "Hello",
	})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestDiscordEmbedConnector_SendsEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		embeds, _ := body["embeds"].([]any)
		if len(embeds) != 1 {
			t.Errorf("expected 1 embed, got %d", len(embeds))
		}
		embed, _ := embeds[0].(map[string]any)
		if embed["title"] != "Alert" {
			t.Errorf("unexpected title: %v", embed["title"])
		}
		if embed["description"] != "Something happened" {
			t.Errorf("unexpected description: %v", embed["description"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         "111",
			"channel_id": "222",
			"timestamp":  "2024-01-01T00:00:00.000Z",
		})
	}))
	defer srv.Close()

	c := &DiscordEmbedConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bot-token"},
		"channel_id":  "222",
		"title":       "Alert",
		"description": "Something happened",
		"color":       16711680,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "111" {
		t.Errorf("expected id=111, got %v", out["id"])
	}
}

func TestDiscordEmbedConnector_WithFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		embeds := body["embeds"].([]any)
		embed := embeds[0].(map[string]any)
		fields, _ := embed["fields"].([]any)
		if len(fields) != 2 {
			t.Errorf("expected 2 fields, got %d", len(fields))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "1", "channel_id": "2", "timestamp": ""})
	}))
	defer srv.Close()

	c := &DiscordEmbedConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bot-token"},
		"channel_id":  "2",
		"fields": []any{
			map[string]any{"name": "Status", "value": "OK"},
			map[string]any{"name": "Region", "value": "us-east-1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiscordEmbedConnector_MissingChannelID(t *testing.T) {
	c := &DiscordEmbedConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bot-token"},
		"title":       "Test",
	})
	if err == nil {
		t.Fatal("expected error for missing channel_id")
	}
}

func TestDiscordSendConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bot mapany" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "1", "channel_id": "2", "timestamp": ""})
	}))
	defer srv.Close()

	c := &DiscordSendConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"token": "mapany"},
		"channel_id":  "2",
		"content":     "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_DiscordConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("discord/send"); err != nil {
		t.Errorf("discord/send not registered: %v", err)
	}
	if _, err := r.Get("discord/embed"); err != nil {
		t.Errorf("discord/embed not registered: %v", err)
	}
}
