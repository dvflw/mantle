package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPagerDutyCreateIncidentConnector_CreatesIncident(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/incidents" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Token token=u+abc" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/vnd.pagerduty+json;version=2" {
			t.Errorf("unexpected Accept: %s", r.Header.Get("Accept"))
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		inc := body["incident"].(map[string]any)
		if inc["title"] != "DB is down" {
			t.Errorf("unexpected title: %v", inc["title"])
		}
		svc := inc["service"].(map[string]any)
		if svc["id"] != "PABC123" {
			t.Errorf("unexpected service id: %v", svc["id"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"incident": map[string]any{
				"id":              "Q2AVLPZB5RX",
				"incident_number": 42,
				"title":           "DB is down",
				"status":          "triggered",
				"urgency":         "high",
				"html_url":        "https://app.pagerduty.com/incidents/Q2AVLPZB5RX",
			},
		})
	}))
	defer srv.Close()

	c := &PagerDutyCreateIncidentConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
		"title":       "DB is down",
		"service_id":  "PABC123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "Q2AVLPZB5RX" {
		t.Errorf("expected id=Q2AVLPZB5RX, got %v", out["id"])
	}
	if out["incident_number"].(int) != 42 {
		t.Errorf("expected incident_number=42, got %v", out["incident_number"])
	}
	if out["status"] != "triggered" {
		t.Errorf("expected status=triggered, got %v", out["status"])
	}
}

func TestPagerDutyCreateIncidentConnector_WithDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		inc := body["incident"].(map[string]any)
		incBody, _ := inc["body"].(map[string]any)
		if incBody["details"] != "Some details" {
			t.Errorf("unexpected details: %v", incBody["details"])
		}
		if inc["urgency"] != "low" {
			t.Errorf("unexpected urgency: %v", inc["urgency"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"incident": map[string]any{
				"id": "QABC", "incident_number": 1,
				"title": "t", "status": "triggered", "urgency": "low", "html_url": "",
			},
		})
	}))
	defer srv.Close()

	c := &PagerDutyCreateIncidentConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
		"title":       "t",
		"service_id":  "SVC1",
		"urgency":     "low",
		"details":     "Some details",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPagerDutyCreateIncidentConnector_FromEmailInCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("From") != "oncall@example.com" {
			t.Errorf("unexpected From header: %s", r.Header.Get("From"))
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"incident": map[string]any{
				"id": "Q1", "incident_number": 1,
				"title": "t", "status": "triggered", "urgency": "high", "html_url": "",
			},
		})
	}))
	defer srv.Close()

	c := &PagerDutyCreateIncidentConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc", "from_email": "oncall@example.com"},
		"title":       "t",
		"service_id":  "SVC1",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPagerDutyCreateIncidentConnector_MissingTitle(t *testing.T) {
	c := &PagerDutyCreateIncidentConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
		"service_id":  "SVC1",
	})
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestPagerDutyCreateIncidentConnector_MissingServiceID(t *testing.T) {
	c := &PagerDutyCreateIncidentConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
		"title":       "t",
	})
	if err == nil {
		t.Fatal("expected error for missing service_id")
	}
}

func TestPagerDutyCreateIncidentConnector_MissingCredential(t *testing.T) {
	c := &PagerDutyCreateIncidentConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"title":      "t",
		"service_id": "SVC1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestPagerDutyCreateIncidentConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"Forbidden"}}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &PagerDutyCreateIncidentConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
		"title":       "t",
		"service_id":  "SVC1",
	})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestPagerDutyResolveConnector_ResolvesIncident(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/incidents/Q2AVLPZB5RX" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		inc := body["incident"].(map[string]any)
		if inc["status"] != "resolved" {
			t.Errorf("expected status=resolved, got %v", inc["status"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"incident": map[string]any{
				"id":     "Q2AVLPZB5RX",
				"status": "resolved",
			},
		})
	}))
	defer srv.Close()

	c := &PagerDutyResolveConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
		"incident_id": "Q2AVLPZB5RX",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "resolved" {
		t.Errorf("expected status=resolved, got %v", out["status"])
	}
}

func TestPagerDutyResolveConnector_MissingIncidentID(t *testing.T) {
	c := &PagerDutyResolveConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
	})
	if err == nil {
		t.Fatal("expected error for missing incident_id")
	}
}

func TestPagerDutyResolveConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"Not Found"}}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := &PagerDutyResolveConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "u+abc"},
		"incident_id": "QMISSING",
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestPagerDutyCreateIncidentConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token token=mapany" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"incident": map[string]any{
				"id": "Q1", "incident_number": 1,
				"title": "t", "status": "triggered", "urgency": "high", "html_url": "",
			},
		})
	}))
	defer srv.Close()

	c := &PagerDutyCreateIncidentConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"token": "mapany"},
		"title":       "t",
		"service_id":  "SVC1",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_PagerDutyConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("pagerduty/create_incident"); err != nil {
		t.Errorf("pagerduty/create_incident not registered: %v", err)
	}
	if _, err := r.Get("pagerduty/resolve"); err != nil {
		t.Errorf("pagerduty/resolve not registered: %v", err)
	}
}
