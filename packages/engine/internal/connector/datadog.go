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

const datadogBaseURL = "https://api.datadoghq.com"

// DatadogSubmitEventConnector submits an event to the Datadog event stream.
type DatadogSubmitEventConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *DatadogSubmitEventConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = datadogBaseURL
	}
	return base + path
}

func (c *DatadogSubmitEventConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	apiKey, appKey, err := extractDatadogCredential(params)
	if err != nil {
		return nil, fmt.Errorf("datadog/submit_event: %w", err)
	}

	title, _ := params["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("datadog/submit_event: title is required")
	}
	text, _ := params["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("datadog/submit_event: text is required")
	}

	event := map[string]any{
		"title": title,
		"text":  text,
	}
	if alertType, ok := params["alert_type"].(string); ok && alertType != "" {
		event["alert_type"] = alertType
	}
	if priority, ok := params["priority"].(string); ok && priority != "" {
		event["priority"] = priority
	}
	if tags, ok := params["tags"].([]any); ok && len(tags) > 0 {
		event["tags"] = tags
	}
	if host, ok := params["host"].(string); ok && host != "" {
		event["host"] = host
	}

	reqJSON, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("datadog/submit_event: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL("/api/v1/events"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("datadog/submit_event: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	if appKey != "" {
		req.Header.Set("DD-APPLICATION-KEY", appKey)
	}

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("datadog/submit_event: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("datadog/submit_event: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("datadog/submit_event: Datadog API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Status string `json:"status"`
		Event  struct {
			ID int64 `json:"id"`
		} `json:"event"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("datadog/submit_event: parsing response: %w", err)
	}

	return map[string]any{
		"status":   result.Status,
		"event_id": result.Event.ID,
	}, nil
}

// DatadogQueryMetricsConnector queries time-series metrics from Datadog.
type DatadogQueryMetricsConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *DatadogQueryMetricsConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = datadogBaseURL
	}
	return base + path
}

func (c *DatadogQueryMetricsConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	apiKey, appKey, err := extractDatadogCredential(params)
	if err != nil {
		return nil, fmt.Errorf("datadog/query_metrics: %w", err)
	}

	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("datadog/query_metrics: query is required")
	}
	from, fromOK := extractInt(params["from"])
	to, toOK := extractInt(params["to"])
	if !fromOK || !toOK {
		return nil, fmt.Errorf("datadog/query_metrics: from and to (Unix timestamps) are required")
	}

	q := url.Values{}
	q.Set("query", query)
	q.Set("from", fmt.Sprintf("%d", from))
	q.Set("to", fmt.Sprintf("%d", to))

	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL("/api/v1/query?"+q.Encode()), nil)
	if err != nil {
		return nil, fmt.Errorf("datadog/query_metrics: creating request: %w", err)
	}
	req.Header.Set("DD-API-KEY", apiKey)
	if appKey != "" {
		req.Header.Set("DD-APPLICATION-KEY", appKey)
	}

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("datadog/query_metrics: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("datadog/query_metrics: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("datadog/query_metrics: Datadog API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Status string `json:"status"`
		Series []any  `json:"series"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("datadog/query_metrics: parsing response: %w", err)
	}

	return map[string]any{
		"status": result.Status,
		"series": result.Series,
		"count":  len(result.Series),
	}, nil
}

// extractDatadogCredential pulls api_key and optional app_key from _credential.
func extractDatadogCredential(params map[string]any) (apiKey, appKey string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	switch cred := raw.(type) {
	case map[string]string:
		apiKey = cred["api_key"]
		appKey = cred["app_key"]
	case map[string]any:
		apiKey, _ = cred["api_key"].(string)
		appKey, _ = cred["app_key"].(string)
	default:
		return "", "", fmt.Errorf("credential is required")
	}
	if apiKey == "" {
		return "", "", fmt.Errorf("credential must contain an 'api_key' field")
	}
	return apiKey, appKey, nil
}
