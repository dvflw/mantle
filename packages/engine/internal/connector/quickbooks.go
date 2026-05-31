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

const quickbooksBaseURL = "https://quickbooks.api.intuit.com/v3/company"

// QuickBooksCreateInvoiceConnector creates an invoice in QuickBooks Online.
type QuickBooksCreateInvoiceConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *QuickBooksCreateInvoiceConnector) apiURL(realmID, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return fmt.Sprintf("%s/%s%s", quickbooksBaseURL, realmID, path)
}

func (c *QuickBooksCreateInvoiceConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	realmID, token, err := extractQuickBooksCredential(params)
	if err != nil {
		return nil, fmt.Errorf("quickbooks/create_invoice: %w", err)
	}

	lineItems, _ := params["line"].([]any)
	if len(lineItems) == 0 {
		return nil, fmt.Errorf("quickbooks/create_invoice: line is required")
	}

	invoice := map[string]any{"Line": lineItems}
	if customer, ok := params["customer_ref"].(map[string]any); ok {
		invoice["CustomerRef"] = customer
	}
	if dueDate, ok := params["due_date"].(string); ok && dueDate != "" {
		invoice["DueDate"] = dueDate
	}
	if txnDate, ok := params["txn_date"].(string); ok && txnDate != "" {
		invoice["TxnDate"] = txnDate
	}
	if memo, ok := params["customer_memo"].(string); ok && memo != "" {
		invoice["CustomerMemo"] = map[string]any{"value": memo}
	}

	return qboPost(ctx, c.Client, c.apiURL(realmID, "/invoice"), token, invoice, "quickbooks/create_invoice")
}

// QuickBooksListInvoicesConnector queries invoices from QuickBooks Online.
type QuickBooksListInvoicesConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *QuickBooksListInvoicesConnector) apiURL(realmID, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return fmt.Sprintf("%s/%s%s", quickbooksBaseURL, realmID, path)
}

func (c *QuickBooksListInvoicesConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	realmID, token, err := extractQuickBooksCredential(params)
	if err != nil {
		return nil, fmt.Errorf("quickbooks/list_invoices: %w", err)
	}

	query := "SELECT * FROM Invoice"
	if where, ok := params["where"].(string); ok && where != "" {
		query += " WHERE " + where
	}
	if orderBy, ok := params["order_by"].(string); ok && orderBy != "" {
		query += " ORDERBY " + orderBy
	}
	maxResults := 20
	if m, ok := extractInt(params["max_results"]); ok && m > 0 {
		maxResults = m
	}
	query += fmt.Sprintf(" MAXRESULTS %d", maxResults)
	if startPos, ok := extractInt(params["start_position"]); ok && startPos > 0 {
		query += fmt.Sprintf(" STARTPOSITION %d", startPos)
	}

	qv := url.Values{}
	qv.Set("query", query)
	qv.Set("minorversion", "65")
	endpoint := c.apiURL(realmID, "/query?"+qv.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("quickbooks/list_invoices: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("quickbooks/list_invoices: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("quickbooks/list_invoices: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quickbooks/list_invoices: QuickBooks API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var result struct {
		QueryResponse struct {
			Invoice    []any `json:"Invoice"`
			TotalCount int   `json:"totalCount"`
		} `json:"QueryResponse"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("quickbooks/list_invoices: parsing response: %w", err)
	}
	invoices := result.QueryResponse.Invoice
	if invoices == nil {
		invoices = []any{}
	}
	return map[string]any{
		"invoices":    invoices,
		"count":       len(invoices),
		"total_count": result.QueryResponse.TotalCount,
	}, nil
}

func extractQuickBooksCredential(params map[string]any) (realmID, token string, err error) {
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

	realmID = cred["realm_id"]
	if realmID == "" {
		return "", "", fmt.Errorf("credential must contain a 'realm_id' field")
	}
	token = cred["access_token"]
	if token == "" {
		return "", "", fmt.Errorf("credential must contain an 'access_token' field")
	}
	return realmID, token, nil
}

func qboPost(ctx context.Context, client *http.Client, endpoint, token string, body map[string]any, action string) (map[string]any, error) {
	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshaling request: %w", action, err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint+"?minorversion=65", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("%s: creating request: %w", action, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", action, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%s: QuickBooks API returned %d: %s", action, resp.StatusCode, truncate(string(respBody), 500))
	}

	return parseJSONBody(respBody, action)
}
