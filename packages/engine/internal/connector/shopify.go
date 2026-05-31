package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const shopifyAPIVersion = "2024-01"

func shopifyAPIURL(baseURL, shop, path string) string {
	if baseURL != "" {
		return baseURL + path
	}
	return fmt.Sprintf("https://%s.myshopify.com/admin/api/%s%s", shop, shopifyAPIVersion, path)
}

// ShopifyListOrdersConnector lists orders from a Shopify store.
type ShopifyListOrdersConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *ShopifyListOrdersConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	shop, token, err := extractShopifyCredential(params)
	if err != nil {
		return nil, fmt.Errorf("shopify/list_orders: %w", err)
	}

	query := url.Values{}
	if status, ok := params["status"].(string); ok && status != "" {
		query.Set("status", status)
	}
	if limit, ok := extractInt(params["limit"]); ok && limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if sinceID, ok := params["since_id"].(string); ok && sinceID != "" {
		query.Set("since_id", sinceID)
	}

	endpoint := shopifyAPIURL(c.baseURL, shop, "/orders.json")
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	out, err := shopifyGet(ctx, c.Client, endpoint, token, "shopify/list_orders")
	if err != nil {
		return nil, err
	}

	orders, _ := out["orders"].([]any)
	out["count"] = len(orders)
	return out, nil
}

// ShopifyListProductsConnector lists products from a Shopify store.
type ShopifyListProductsConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *ShopifyListProductsConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	shop, token, err := extractShopifyCredential(params)
	if err != nil {
		return nil, fmt.Errorf("shopify/list_products: %w", err)
	}

	query := url.Values{}
	if limit, ok := extractInt(params["limit"]); ok && limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if productType, ok := params["product_type"].(string); ok && productType != "" {
		query.Set("product_type", productType)
	}
	if vendor, ok := params["vendor"].(string); ok && vendor != "" {
		query.Set("vendor", vendor)
	}

	endpoint := shopifyAPIURL(c.baseURL, shop, "/products.json")
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	out, err := shopifyGet(ctx, c.Client, endpoint, token, "shopify/list_products")
	if err != nil {
		return nil, err
	}

	products, _ := out["products"].([]any)
	out["count"] = len(products)
	return out, nil
}

// ShopifyCreateOrderConnector creates an order in a Shopify store.
type ShopifyCreateOrderConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *ShopifyCreateOrderConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	shop, token, err := extractShopifyCredential(params)
	if err != nil {
		return nil, fmt.Errorf("shopify/create_order: %w", err)
	}

	lineItems, _ := params["line_items"].([]any)
	if len(lineItems) == 0 {
		return nil, fmt.Errorf("shopify/create_order: line_items is required")
	}

	order := map[string]any{"line_items": lineItems}
	if email, ok := params["email"].(string); ok && email != "" {
		order["email"] = email
	}
	if customer, ok := params["customer"].(map[string]any); ok {
		order["customer"] = customer
	}
	if note, ok := params["note"].(string); ok && note != "" {
		order["note"] = note
	}

	reqJSON, err := json.Marshal(map[string]any{"order": order})
	if err != nil {
		return nil, fmt.Errorf("shopify/create_order: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", shopifyAPIURL(c.baseURL, shop, "/orders.json"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("shopify/create_order: creating request: %w", err)
	}
	req.Header.Set("X-Shopify-Access-Token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("shopify/create_order: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("shopify/create_order: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shopify/create_order: Shopify API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "shopify/create_order")
}

func extractShopifyCredential(params map[string]any) (shop, token string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var cred map[string]string
	switch v := raw.(type) {
	case map[string]string:
		cred = v
	case map[string]any:
		cred = make(map[string]string, len(v))
		for k, val := range v {
			if s, ok := val.(string); ok {
				cred[k] = s
			}
		}
	default:
		return "", "", fmt.Errorf("credential is required")
	}

	shop = cred["shop"]
	if shop == "" {
		return "", "", fmt.Errorf("credential must contain a 'shop' field (subdomain only, e.g. mystore)")
	}
	token = cred["token"]
	if token == "" {
		return "", "", fmt.Errorf("credential must contain a 'token' field")
	}
	return shop, token, nil
}

func shopifyGet(ctx context.Context, client *http.Client, endpoint, token, action string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: creating request: %w", action, err)
	}
	req.Header.Set("X-Shopify-Access-Token", token)

	resp, err := httpClient(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", action, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: Shopify API returned %d: %s", action, resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, action)
}
