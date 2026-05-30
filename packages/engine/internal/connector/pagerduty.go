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

const pagerdutyBaseURL = "https://api.pagerduty.com"

// PagerDutyCreateIncidentConnector creates a PagerDuty incident.
type PagerDutyCreateIncidentConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *PagerDutyCreateIncidentConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = pagerdutyBaseURL
	}
	return base + path
}

func (c *PagerDutyCreateIncidentConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, fromEmail, err := extractPagerDutyCredential(params)
	if err != nil {
		return nil, fmt.Errorf("pagerduty/create_incident: %w", err)
	}

	title, _ := params["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("pagerduty/create_incident: title is required")
	}
	serviceID, _ := params["service_id"].(string)
	if serviceID == "" {
		return nil, fmt.Errorf("pagerduty/create_incident: service_id is required")
	}

	incident := map[string]any{
		"type":    "incident",
		"title":   title,
		"service": map[string]any{"id": serviceID, "type": "service_reference"},
	}
	if urgency, ok := params["urgency"].(string); ok && urgency != "" {
		incident["urgency"] = urgency
	}
	if details, ok := params["details"].(string); ok && details != "" {
		incident["body"] = map[string]any{
			"type":    "incident_body",
			"details": details,
		}
	}

	body := map[string]any{"incident": incident}
	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("pagerduty/create_incident: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL("/incidents"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("pagerduty/create_incident: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token token="+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	if fromEmail != "" {
		req.Header.Set("From", fromEmail)
	} else if fe, ok := params["from_email"].(string); ok && fe != "" {
		req.Header.Set("From", fe)
	}

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("pagerduty/create_incident: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("pagerduty/create_incident: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("pagerduty/create_incident: PagerDuty API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Incident struct {
			ID             string `json:"id"`
			IncidentNumber int    `json:"incident_number"`
			Title          string `json:"title"`
			Status         string `json:"status"`
			Urgency        string `json:"urgency"`
			HTMLURL        string `json:"html_url"`
		} `json:"incident"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("pagerduty/create_incident: parsing response: %w", err)
	}

	return map[string]any{
		"id":              result.Incident.ID,
		"incident_number": result.Incident.IncidentNumber,
		"title":           result.Incident.Title,
		"status":          result.Incident.Status,
		"urgency":         result.Incident.Urgency,
		"html_url":        result.Incident.HTMLURL,
	}, nil
}

// PagerDutyResolveConnector resolves a PagerDuty incident.
type PagerDutyResolveConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *PagerDutyResolveConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = pagerdutyBaseURL
	}
	return base + path
}

func (c *PagerDutyResolveConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, fromEmail, err := extractPagerDutyCredential(params)
	if err != nil {
		return nil, fmt.Errorf("pagerduty/resolve: %w", err)
	}

	incidentID, _ := params["incident_id"].(string)
	if incidentID == "" {
		return nil, fmt.Errorf("pagerduty/resolve: incident_id is required")
	}

	// from_email is required for REST API key auth: PagerDuty rejects incident
	// updates without a From header when using non-user-context credentials.
	if fromEmail == "" {
		fromEmail, _ = params["from_email"].(string)
	}
	if fromEmail == "" {
		return nil, fmt.Errorf("pagerduty/resolve: from_email is required (set in credential or as param)")
	}

	body := map[string]any{
		"incident": map[string]any{
			"type":   "incident",
			"status": "resolved",
		},
	}
	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("pagerduty/resolve: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/incidents/%s", url.PathEscape(incidentID))
	req, err := http.NewRequestWithContext(ctx, "PUT", c.apiURL(path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("pagerduty/resolve: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Token token="+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	req.Header.Set("From", fromEmail)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("pagerduty/resolve: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("pagerduty/resolve: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pagerduty/resolve: PagerDuty API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Incident struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"incident"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("pagerduty/resolve: parsing response: %w", err)
	}

	return map[string]any{
		"id":     result.Incident.ID,
		"status": result.Incident.Status,
	}, nil
}

// extractPagerDutyCredential extracts the API token and optional from_email from _credential.
func extractPagerDutyCredential(params map[string]any) (token, fromEmail string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	switch cred := raw.(type) {
	case map[string]string:
		token = cred["token"]
		fromEmail = cred["from_email"]
	case map[string]any:
		token, _ = cred["token"].(string)
		fromEmail, _ = cred["from_email"].(string)
	default:
		return "", "", fmt.Errorf("credential is required")
	}
	if token == "" {
		return "", "", fmt.Errorf("credential must contain a 'token' field")
	}
	return token, fromEmail, nil
}
