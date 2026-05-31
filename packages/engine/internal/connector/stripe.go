package connector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const stripeBaseURL = "https://api.stripe.com/v1"

// StripeCreateChargeConnector creates a one-time charge via the Stripe API.
type StripeCreateChargeConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *StripeCreateChargeConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = stripeBaseURL
	}
	return base + path
}

func (c *StripeCreateChargeConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	apiKey, err := extractStripeCredential(params)
	if err != nil {
		return nil, fmt.Errorf("stripe/create_charge: %w", err)
	}

	amount, ok := extractInt(params["amount"])
	if !ok || amount <= 0 {
		return nil, fmt.Errorf("stripe/create_charge: amount is required (positive integer in smallest currency unit)")
	}
	currency, _ := params["currency"].(string)
	if currency == "" {
		return nil, fmt.Errorf("stripe/create_charge: currency is required")
	}

	form := url.Values{}
	form.Set("amount", fmt.Sprintf("%d", amount))
	form.Set("currency", strings.ToLower(currency))
	if source, ok := params["source"].(string); ok && source != "" {
		form.Set("source", source)
	}
	if customer, ok := params["customer"].(string); ok && customer != "" {
		form.Set("customer", customer)
	}
	if desc, ok := params["description"].(string); ok && desc != "" {
		form.Set("description", desc)
	}

	return stripePost(ctx, c.Client, c.apiURL("/charges"), apiKey, form, "stripe/create_charge")
}

// StripeCreateCustomerConnector creates a customer object in Stripe.
type StripeCreateCustomerConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *StripeCreateCustomerConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = stripeBaseURL
	}
	return base + path
}

func (c *StripeCreateCustomerConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	apiKey, err := extractStripeCredential(params)
	if err != nil {
		return nil, fmt.Errorf("stripe/create_customer: %w", err)
	}

	form := url.Values{}
	if email, ok := params["email"].(string); ok && email != "" {
		form.Set("email", email)
	}
	if name, ok := params["name"].(string); ok && name != "" {
		form.Set("name", name)
	}
	if desc, ok := params["description"].(string); ok && desc != "" {
		form.Set("description", desc)
	}
	if phone, ok := params["phone"].(string); ok && phone != "" {
		form.Set("phone", phone)
	}

	return stripePost(ctx, c.Client, c.apiURL("/customers"), apiKey, form, "stripe/create_customer")
}

// StripeCreateRefundConnector refunds a charge via the Stripe API.
type StripeCreateRefundConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *StripeCreateRefundConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = stripeBaseURL
	}
	return base + path
}

func (c *StripeCreateRefundConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	apiKey, err := extractStripeCredential(params)
	if err != nil {
		return nil, fmt.Errorf("stripe/create_refund: %w", err)
	}

	charge, _ := params["charge"].(string)
	if charge == "" {
		return nil, fmt.Errorf("stripe/create_refund: charge is required")
	}

	form := url.Values{}
	form.Set("charge", charge)
	if amount, ok := extractInt(params["amount"]); ok && amount > 0 {
		form.Set("amount", fmt.Sprintf("%d", amount))
	}
	if reason, ok := params["reason"].(string); ok && reason != "" {
		form.Set("reason", reason)
	}

	return stripePost(ctx, c.Client, c.apiURL("/refunds"), apiKey, form, "stripe/create_refund")
}

func extractStripeCredential(params map[string]any) (string, error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var apiKey string
	switch cred := raw.(type) {
	case map[string]string:
		apiKey = cred["api_key"]
	case map[string]any:
		apiKey, _ = cred["api_key"].(string)
	default:
		return "", fmt.Errorf("credential is required")
	}
	if apiKey == "" {
		return "", fmt.Errorf("credential must contain an 'api_key' field")
	}
	return apiKey, nil
}

// stripePost sends a form-encoded POST to the Stripe API and returns the parsed JSON response.
func stripePost(ctx context.Context, client *http.Client, endpoint, apiKey string, form url.Values, action string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%s: creating request: %w", action, err)
	}
	req.SetBasicAuth(apiKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", action, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: Stripe API returned %d: %s", action, resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, action)
}
