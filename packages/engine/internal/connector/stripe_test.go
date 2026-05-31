package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripeCreateChargeConnector_CreatesCharge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/charges" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		user, _, ok := r.BasicAuth()
		if !ok || user != "sk_test_key" {
			t.Errorf("unexpected auth user: %s", user)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("amount") != "2000" {
			t.Errorf("unexpected amount: %s", r.FormValue("amount"))
		}
		if r.FormValue("currency") != "usd" {
			t.Errorf("unexpected currency: %s", r.FormValue("currency"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "ch_123", "status": "succeeded", "amount": 2000})
	}))
	defer srv.Close()

	c := &StripeCreateChargeConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "sk_test_key"},
		"amount":      2000,
		"currency":    "usd",
		"source":      "tok_visa",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "ch_123" {
		t.Errorf("expected id=ch_123, got %v", out["id"])
	}
}

func TestStripeCreateChargeConnector_MissingAmount(t *testing.T) {
	c := &StripeCreateChargeConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "sk_test"},
		"currency":    "usd",
	})
	if err == nil {
		t.Fatal("expected error for missing amount")
	}
}

func TestStripeCreateChargeConnector_MissingCurrency(t *testing.T) {
	c := &StripeCreateChargeConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "sk_test"},
		"amount":      2000,
	})
	if err == nil {
		t.Fatal("expected error for missing currency")
	}
}

func TestStripeCreateChargeConnector_MissingCredential(t *testing.T) {
	c := &StripeCreateChargeConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"amount":   2000,
		"currency": "usd",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestStripeCreateChargeConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"No such token"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c := &StripeCreateChargeConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "sk_bad"},
		"amount":      500,
		"currency":    "usd",
		"source":      "tok_bad",
	})
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

func TestStripeCreateCustomerConnector_CreatesCustomer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/customers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		r.ParseForm()
		if r.FormValue("email") != "alice@example.com" {
			t.Errorf("unexpected email: %s", r.FormValue("email"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "cus_abc", "email": "alice@example.com"})
	}))
	defer srv.Close()

	c := &StripeCreateCustomerConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "sk_test"},
		"email":       "alice@example.com",
		"name":        "Alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "cus_abc" {
		t.Errorf("expected id=cus_abc, got %v", out["id"])
	}
}

func TestStripeCreateRefundConnector_CreatesRefund(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("charge") != "ch_123" {
			t.Errorf("unexpected charge: %s", r.FormValue("charge"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "re_abc", "status": "succeeded"})
	}))
	defer srv.Close()

	c := &StripeCreateRefundConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "sk_test"},
		"charge":      "ch_123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "re_abc" {
		t.Errorf("expected id=re_abc, got %v", out["id"])
	}
}

func TestStripeCreateRefundConnector_MissingCharge(t *testing.T) {
	c := &StripeCreateRefundConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"api_key": "sk_test"},
	})
	if err == nil {
		t.Fatal("expected error for missing charge")
	}
}

func TestStripeCreateChargeConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ := r.BasicAuth()
		if user != "sk_mapkey" {
			t.Errorf("unexpected auth user: %s", user)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "ch_1", "status": "succeeded"})
	}))
	defer srv.Close()

	c := &StripeCreateChargeConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"api_key": "sk_mapkey"},
		"amount":      100,
		"currency":    "usd",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_StripeConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"stripe/create_charge", "stripe/create_customer", "stripe/create_refund"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
