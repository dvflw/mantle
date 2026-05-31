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

const salesforceAPIVersion = "v59.0"

// extractSalesforceCredential extracts access_token and instance_url from _credential.
// Accepts both map[string]string and map[string]any credential shapes.
func extractSalesforceCredential(params map[string]any) (instanceURL, token string, err error) {
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

	instanceURL = cred["instance_url"]
	if instanceURL == "" {
		return "", "", fmt.Errorf("credential must contain an 'instance_url' field")
	}
	token = cred["access_token"]
	if token == "" {
		return "", "", fmt.Errorf("credential must contain an 'access_token' field")
	}
	return instanceURL, token, nil
}

// SalesforceQueryConnector executes a SOQL query against Salesforce.
type SalesforceQueryConnector struct {
	Client  *http.Client
	baseURL string // override for testing (replaces instanceURL+/services/data/v59.0)
}

func (c *SalesforceQueryConnector) dataURL(instanceURL, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return instanceURL + "/services/data/" + salesforceAPIVersion + path
}

func (c *SalesforceQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	instanceURL, token, err := extractSalesforceCredential(params)
	if err != nil {
		return nil, fmt.Errorf("salesforce/query: %w", err)
	}

	soql, _ := params["soql"].(string)
	if soql == "" {
		return nil, fmt.Errorf("salesforce/query: soql is required")
	}

	endpoint := c.dataURL(instanceURL, "/query?q="+url.QueryEscape(soql))
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("salesforce/query: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("salesforce/query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("salesforce/query: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("salesforce/query: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var result struct {
		Records   []any `json:"records"`
		TotalSize int   `json:"totalSize"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("salesforce/query: parsing response: %w", err)
	}

	return map[string]any{
		"records":    result.Records,
		"total_size": result.TotalSize,
		"count":      len(result.Records),
	}, nil
}

// SalesforceCreateRecordConnector creates a Salesforce sObject record.
type SalesforceCreateRecordConnector struct {
	Client  *http.Client
	baseURL string // override for testing (replaces instanceURL+/services/data/v59.0)
}

func (c *SalesforceCreateRecordConnector) dataURL(instanceURL, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return instanceURL + "/services/data/" + salesforceAPIVersion + path
}

func (c *SalesforceCreateRecordConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	instanceURL, token, err := extractSalesforceCredential(params)
	if err != nil {
		return nil, fmt.Errorf("salesforce/create_record: %w", err)
	}

	objectType, _ := params["object_type"].(string)
	if objectType == "" {
		return nil, fmt.Errorf("salesforce/create_record: object_type is required")
	}
	fields, ok := params["fields"].(map[string]any)
	if !ok || len(fields) == 0 {
		return nil, fmt.Errorf("salesforce/create_record: fields is required")
	}

	reqJSON, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("salesforce/create_record: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/sobjects/%s", url.PathEscape(objectType))
	req, err := http.NewRequestWithContext(ctx, "POST", c.dataURL(instanceURL, path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("salesforce/create_record: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("salesforce/create_record: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("salesforce/create_record: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("salesforce/create_record: API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	return parseJSONBody(respBody, "salesforce/create_record")
}
