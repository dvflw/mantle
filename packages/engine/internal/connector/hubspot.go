package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const hubspotBaseURL = "https://api.hubapi.com"

// HubSpotCreateContactConnector creates a contact in HubSpot CRM.
type HubSpotCreateContactConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *HubSpotCreateContactConnector) apiURL(path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return hubspotBaseURL + path
}

func (c *HubSpotCreateContactConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("hubspot/create_contact: %w", err)
	}

	email, _ := params["email"].(string)
	if email == "" {
		return nil, fmt.Errorf("hubspot/create_contact: email is required")
	}

	properties := map[string]any{"email": email}
	if v, ok := params["firstname"].(string); ok && v != "" {
		properties["firstname"] = v
	}
	if v, ok := params["lastname"].(string); ok && v != "" {
		properties["lastname"] = v
	}
	if v, ok := params["phone"].(string); ok && v != "" {
		properties["phone"] = v
	}
	if v, ok := params["company"].(string); ok && v != "" {
		properties["company"] = v
	}

	bodyMap := map[string]any{"properties": properties}
	reqJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("hubspot/create_contact: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL("/crm/v3/objects/contacts"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("hubspot/create_contact: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("hubspot/create_contact: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("hubspot/create_contact: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hubspot/create_contact: API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	return parseJSONBody(respBody, "hubspot/create_contact")
}

// HubSpotSearchContactsConnector searches contacts in HubSpot CRM.
type HubSpotSearchContactsConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *HubSpotSearchContactsConnector) apiURL(path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return hubspotBaseURL + path
}

func (c *HubSpotSearchContactsConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("hubspot/search_contacts: %w", err)
	}

	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("hubspot/search_contacts: query is required")
	}

	limit := 10
	if l, ok := extractInt(params["limit"]); ok && l > 0 {
		limit = l
	}

	bodyMap := map[string]any{
		"query":      query,
		"limit":      limit,
		"properties": []string{"email", "firstname", "lastname", "phone", "company"},
	}
	reqJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("hubspot/search_contacts: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL("/crm/v3/objects/contacts/search"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("hubspot/search_contacts: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("hubspot/search_contacts: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("hubspot/search_contacts: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hubspot/search_contacts: API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Results []any `json:"results"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("hubspot/search_contacts: parsing response: %w", err)
	}

	return map[string]any{
		"results": result.Results,
		"count":   len(result.Results),
	}, nil
}
