package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatadogSubmitEventConnector_SubmitsEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/events" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("DD-API-KEY") != "dd-api-key" {
			t.Errorf("unexpected DD-API-KEY: %s", r.Header.Get("DD-API-KEY"))
		}
		if r.Header.Get("DD-APPLICATION-KEY") != "dd-app-key" {
			t.Errorf("unexpected DD-APPLICATION-KEY: %s", r.Header.Get("DD-APPLICATION-KEY"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["title"] != "Deploy completed" {
			t.Errorf("unexpected title: %v", body["title"])
		}
		if body["text"] != "v1.2.3 deployed" {
			t.Errorf("unexpected text: %v", body["text"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"event":  map[string]any{"id": int64(12345)},
		})
	}))
	defer srv.Close()

	c := &DatadogSubmitEventConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "dd-api-key", "app_key": "dd-app-key"},
		"title":       "Deploy completed",
		"text":        "v1.2.3 deployed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", out["status"])
	}
}

func TestDatadogSubmitEventConnector_WithOptionalFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["alert_type"] != "error" {
			t.Errorf("unexpected alert_type: %v", body["alert_type"])
		}
		if body["priority"] != "high" {
			t.Errorf("unexpected priority: %v", body["priority"])
		}
		tags, _ := body["tags"].([]any)
		if len(tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(tags))
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "event": map[string]any{"id": int64(1)}})
	}))
	defer srv.Close()

	c := &DatadogSubmitEventConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key"},
		"title":       "t",
		"text":        "txt",
		"alert_type":  "error",
		"priority":    "high",
		"tags":        []any{"env:prod", "service:api"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDatadogSubmitEventConnector_MissingTitle(t *testing.T) {
	c := &DatadogSubmitEventConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key"},
		"text":        "txt",
	})
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestDatadogSubmitEventConnector_MissingText(t *testing.T) {
	c := &DatadogSubmitEventConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key"},
		"title":       "t",
	})
	if err == nil {
		t.Fatal("expected error for missing text")
	}
}

func TestDatadogSubmitEventConnector_MissingCredential(t *testing.T) {
	c := &DatadogSubmitEventConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"title": "t",
		"text":  "txt",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestDatadogSubmitEventConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errors":["API key is invalid"]}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &DatadogSubmitEventConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "bad"},
		"title":       "t",
		"text":        "txt",
	})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestDatadogQueryMetricsConnector_QueriesMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		q := r.URL.Query()
		if q.Get("query") != "avg:system.cpu.user{*}" {
			t.Errorf("unexpected query: %s", q.Get("query"))
		}
		if q.Get("from") != "1700000000" {
			t.Errorf("unexpected from: %s", q.Get("from"))
		}
		if q.Get("to") != "1700003600" {
			t.Errorf("unexpected to: %s", q.Get("to"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"series": []any{
				map[string]any{"metric": "system.cpu.user", "pointlist": []any{}},
			},
		})
	}))
	defer srv.Close()

	c := &DatadogQueryMetricsConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key"},
		"query":       "avg:system.cpu.user{*}",
		"from":        1700000000,
		"to":          1700003600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", out["status"])
	}
	if out["count"].(int) != 1 {
		t.Errorf("expected count=1, got %v", out["count"])
	}
}

func TestDatadogQueryMetricsConnector_MissingQuery(t *testing.T) {
	c := &DatadogQueryMetricsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key"},
		"from":        1700000000,
		"to":          1700003600,
	})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestDatadogQueryMetricsConnector_MissingFromTo(t *testing.T) {
	c := &DatadogQueryMetricsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "key"},
		"query":       "avg:system.cpu.user{*}",
	})
	if err == nil {
		t.Fatal("expected error for missing from/to")
	}
}

func TestDatadogSubmitEventConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("DD-API-KEY") != "mapkey" {
			t.Errorf("unexpected DD-API-KEY: %s", r.Header.Get("DD-API-KEY"))
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "event": map[string]any{"id": int64(1)}})
	}))
	defer srv.Close()

	c := &DatadogSubmitEventConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"api_key": "mapkey"},
		"title":       "t",
		"text":        "txt",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_DatadogConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("datadog/submit_event"); err != nil {
		t.Errorf("datadog/submit_event not registered: %v", err)
	}
	if _, err := r.Get("datadog/query_metrics"); err != nil {
		t.Errorf("datadog/query_metrics not registered: %v", err)
	}
}
