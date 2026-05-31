package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTeamsSendMessageConnector_SendsMessage(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1"))
	}))
	defer srv.Close()

	c := &TeamsSendMessageConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"webhook_url": "https://teams.example.com/webhook"},
		"text":        "Hello, Teams!",
		"title":       "Alert",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
	if gotBody["text"] != "Hello, Teams!" {
		t.Errorf("text = %v", gotBody["text"])
	}
	if gotBody["title"] != "Alert" {
		t.Errorf("title = %v", gotBody["title"])
	}
}

func TestTeamsSendMessageConnector_MissingCredential(t *testing.T) {
	c := &TeamsSendMessageConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"text": "Hello",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestTeamsSendMessageConnector_MissingWebhookURL(t *testing.T) {
	c := &TeamsSendMessageConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{},
		"text":        "Hello",
	})
	if err == nil {
		t.Fatal("expected error for missing webhook_url")
	}
}

func TestTeamsSendMessageConnector_MissingText(t *testing.T) {
	c := &TeamsSendMessageConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"webhook_url": "https://teams.example.com/webhook"},
	})
	if err == nil {
		t.Fatal("expected error for missing text")
	}
}

func TestTeamsSendAdaptiveCardConnector_SendsCard(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1"))
	}))
	defer srv.Close()

	card := map[string]any{
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body":    []any{map[string]any{"type": "TextBlock", "text": "Hello"}},
	}

	c := &TeamsSendAdaptiveCardConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"webhook_url": "https://teams.example.com/webhook"},
		"card":        card,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
	if gotBody["type"] != "message" {
		t.Errorf("type = %v, want message", gotBody["type"])
	}
	attachments, ok := gotBody["attachments"].([]any)
	if !ok || len(attachments) == 0 {
		t.Fatalf("expected attachments, got %v", gotBody["attachments"])
	}
	att := attachments[0].(map[string]any)
	if att["contentType"] != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("contentType = %v", att["contentType"])
	}
}

func TestTeamsSendAdaptiveCardConnector_MissingCard(t *testing.T) {
	c := &TeamsSendAdaptiveCardConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"webhook_url": "https://teams.example.com/webhook"},
	})
	if err == nil {
		t.Fatal("expected error for missing card")
	}
}

func TestRegistry_TeamsConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"teams/send_message", "teams/send_adaptive_card"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
