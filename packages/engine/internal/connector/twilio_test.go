package connector

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTwilioSMSConnector_SendsSMS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/Messages.json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		// Verify basic auth.
		user, pass, ok := r.BasicAuth()
		if !ok || user != "ACtest123" || pass != "secret" {
			t.Errorf("unexpected basic auth: %s/%s", user, pass)
		}

		r.ParseForm()
		if r.FormValue("To") != "+15551234567" {
			t.Errorf("unexpected To: %s", r.FormValue("To"))
		}
		if r.FormValue("From") != "+15559876543" {
			t.Errorf("unexpected From: %s", r.FormValue("From"))
		}
		if r.FormValue("Body") != "Hello!" {
			t.Errorf("unexpected Body: %s", r.FormValue("Body"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{
			"sid": "SM123",
			"status": "queued",
			"to": "+15551234567",
			"from": "+15559876543",
			"body": "Hello!"
		}`))
	}))
	defer srv.Close()

	c := &TwilioSMSConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "+15551234567",
		"from":        "+15559876543",
		"body":        "Hello!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["sid"] != "SM123" {
		t.Errorf("expected sid=SM123, got %v", out["sid"])
	}
	if out["status"] != "queued" {
		t.Errorf("expected status=queued, got %v", out["status"])
	}
}

func TestTwilioSMSConnector_MissingTo(t *testing.T) {
	c := &TwilioSMSConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"from":        "+15559876543",
		"body":        "Hello!",
	})
	if err == nil {
		t.Fatal("expected error for missing to")
	}
}

func TestTwilioSMSConnector_MissingFrom(t *testing.T) {
	c := &TwilioSMSConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "+15551234567",
		"body":        "Hello!",
	})
	if err == nil {
		t.Fatal("expected error for missing from")
	}
}

func TestTwilioSMSConnector_MissingBody(t *testing.T) {
	c := &TwilioSMSConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "+15551234567",
		"from":        "+15559876543",
	})
	if err == nil {
		t.Fatal("expected error for missing body")
	}
}

func TestTwilioSMSConnector_MissingCredential(t *testing.T) {
	c := &TwilioSMSConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"to":   "+15551234567",
		"from": "+15559876543",
		"body": "Hello!",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestTwilioSMSConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":21211,"message":"The 'To' number is not a valid phone number"}`))
	}))
	defer srv.Close()

	c := &TwilioSMSConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "invalid",
		"from":        "+15559876543",
		"body":        "Hello!",
	})
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

func TestTwilioCallConnector_CallWithURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/Calls.json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		r.ParseForm()
		if r.FormValue("Url") != "https://handler.twilio.com/twiml/EX1234" {
			t.Errorf("unexpected Url: %s", r.FormValue("Url"))
		}
		if r.FormValue("Twiml") != "" {
			t.Errorf("Twiml should be empty when Url is set")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"CA123","status":"queued","to":"+15551234567","from":"+15559876543"}`))
	}))
	defer srv.Close()

	c := &TwilioCallConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "+15551234567",
		"from":        "+15559876543",
		"url":         "https://handler.twilio.com/twiml/EX1234",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["sid"] != "CA123" {
		t.Errorf("expected sid=CA123, got %v", out["sid"])
	}
}

func TestTwilioCallConnector_CallWithTwiml(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		twiml := r.FormValue("Twiml")
		if !strings.Contains(twiml, "<Say>") {
			t.Errorf("expected <Say> in Twiml: %s", twiml)
		}
		if r.FormValue("Url") != "" {
			t.Errorf("Url should be empty when Twiml is set")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"CA456","status":"queued","to":"+15551234567","from":"+15559876543"}`))
	}))
	defer srv.Close()

	c := &TwilioCallConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "+15551234567",
		"from":        "+15559876543",
		"twiml":       "<Response><Say>Hello!</Say></Response>",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTwilioCallConnector_MissingURLAndTwiml(t *testing.T) {
	c := &TwilioCallConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "+15551234567",
		"from":        "+15559876543",
	})
	if err == nil {
		t.Fatal("expected error when neither url nor twiml is set")
	}
}

func TestTwilioCallConnector_MissingTo(t *testing.T) {
	c := &TwilioCallConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"from":        "+15559876543",
		"url":         "https://example.com/twiml",
	})
	if err == nil {
		t.Fatal("expected error for missing to")
	}
}

func TestTwilioSMSConnector_AccountSIDInPath(t *testing.T) {
	// Verify that the account SID is included in the URL path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, url.PathEscape("ACtest123")) {
			t.Errorf("account SID not found in path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM1","status":"queued","to":"+1","from":"+2","body":"hi"}`))
	}))
	defer srv.Close()

	c := &TwilioSMSConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"account_sid": "ACtest123", "auth_token": "secret"},
		"to":          "+1",
		"from":        "+2",
		"body":        "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTwilioSMSConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "ACmap" || pass != "maptoken" {
			t.Errorf("unexpected basic auth: %s/%s", user, pass)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM1","status":"queued","to":"+1","from":"+2","body":"hi"}`))
	}))
	defer srv.Close()

	c := &TwilioSMSConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"account_sid": "ACmap", "auth_token": "maptoken"},
		"to":          "+1",
		"from":        "+2",
		"body":        "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_TwilioConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("twilio/sms"); err != nil {
		t.Errorf("twilio/sms not registered: %v", err)
	}
	if _, err := r.Get("twilio/call"); err != nil {
		t.Errorf("twilio/call not registered: %v", err)
	}
}
