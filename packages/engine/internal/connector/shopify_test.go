package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShopifyListOrdersConnector_ListsOrders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/orders.json" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Shopify-Access-Token") != "shpat_token" {
			t.Errorf("unexpected token: %s", r.Header.Get("X-Shopify-Access-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"orders": []any{
				map[string]any{"id": 1001, "name": "#1001"},
				map[string]any{"id": 1002, "name": "#1002"},
			},
		})
	}))
	defer srv.Close()

	c := &ShopifyListOrdersConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"shop": "mystore", "token": "shpat_token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestShopifyListOrdersConnector_WithStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "open" {
			t.Errorf("expected status=open, got %s", r.URL.Query().Get("status"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"orders": []any{}})
	}))
	defer srv.Close()

	c := &ShopifyListOrdersConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"shop": "mystore", "token": "tok"},
		"status":      "open",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestShopifyListOrdersConnector_MissingCredential(t *testing.T) {
	c := &ShopifyListOrdersConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestShopifyListOrdersConnector_MissingShop(t *testing.T) {
	c := &ShopifyListOrdersConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing shop")
	}
}

func TestShopifyListProductsConnector_ListsProducts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"products": []any{
				map[string]any{"id": 2001, "title": "Widget"},
			},
		})
	}))
	defer srv.Close()

	c := &ShopifyListProductsConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"shop": "mystore", "token": "tok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 1 {
		t.Errorf("expected count=1, got %v", out["count"])
	}
}

func TestShopifyCreateOrderConnector_CreatesOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/orders.json" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		order, _ := body["order"].(map[string]any)
		lineItems, _ := order["line_items"].([]any)
		if len(lineItems) != 1 {
			t.Errorf("expected 1 line item, got %d", len(lineItems))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"order": map[string]any{"id": 3001, "name": "#3001"}})
	}))
	defer srv.Close()

	c := &ShopifyCreateOrderConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"shop": "mystore", "token": "tok"},
		"line_items":  []any{map[string]any{"variant_id": 1, "quantity": 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestShopifyCreateOrderConnector_MissingLineItems(t *testing.T) {
	c := &ShopifyCreateOrderConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"shop": "mystore", "token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for missing line_items")
	}
}

func TestShopifyListOrdersConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errors":"[API] Invalid API key or access token"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &ShopifyListOrdersConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"shop": "mystore", "token": "bad"},
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestShopifyListOrdersConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Shopify-Access-Token") != "maptoken" {
			t.Errorf("unexpected token: %s", r.Header.Get("X-Shopify-Access-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"orders": []any{}})
	}))
	defer srv.Close()

	c := &ShopifyListOrdersConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"shop": "mystore", "token": "maptoken"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_ShopifyConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"shopify/list_orders", "shopify/list_products", "shopify/create_order"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
