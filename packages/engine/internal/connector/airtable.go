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

const airtableBaseURL = "https://api.airtable.com/v0"

// AirtableListConnector lists records from an Airtable table.
type AirtableListConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *AirtableListConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = airtableBaseURL
	}
	return base + path
}

func (c *AirtableListConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("airtable/list: %w", err)
	}

	baseID, _ := params["base_id"].(string)
	if baseID == "" {
		return nil, fmt.Errorf("airtable/list: base_id is required")
	}
	tableID, _ := params["table_id"].(string)
	if tableID == "" {
		return nil, fmt.Errorf("airtable/list: table_id is required")
	}

	path := fmt.Sprintf("/%s/%s", url.PathEscape(baseID), url.PathEscape(tableID))
	reqURL := c.apiURL(path)

	q := url.Values{}
	if maxRecords, ok := extractInt(params["max_records"]); ok && maxRecords > 0 {
		q.Set("maxRecords", fmt.Sprintf("%d", maxRecords))
	}
	if formula, ok := params["filter_by_formula"].(string); ok && formula != "" {
		q.Set("filterByFormula", formula)
	}
	if view, ok := params["view"].(string); ok && view != "" {
		q.Set("view", view)
	}
	if cursor, ok := params["offset"].(string); ok && cursor != "" {
		q.Set("offset", cursor)
	}
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("airtable/list: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("airtable/list: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("airtable/list: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("airtable/list: Airtable API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Records []any  `json:"records"`
		Offset  string `json:"offset"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("airtable/list: parsing response: %w", err)
	}

	out := map[string]any{
		"records": result.Records,
		"count":   len(result.Records),
	}
	if result.Offset != "" {
		out["offset"] = result.Offset
	}
	return out, nil
}

// AirtableCreateRecordConnector creates a record in an Airtable table.
type AirtableCreateRecordConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *AirtableCreateRecordConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = airtableBaseURL
	}
	return base + path
}

func (c *AirtableCreateRecordConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("airtable/create_record: %w", err)
	}

	baseID, _ := params["base_id"].(string)
	if baseID == "" {
		return nil, fmt.Errorf("airtable/create_record: base_id is required")
	}
	tableID, _ := params["table_id"].(string)
	if tableID == "" {
		return nil, fmt.Errorf("airtable/create_record: table_id is required")
	}

	fields, _ := params["fields"].(map[string]any)
	if fields == nil {
		fields = map[string]any{}
	}

	body := map[string]any{"fields": fields}
	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("airtable/create_record: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/%s/%s", url.PathEscape(baseID), url.PathEscape(tableID))
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("airtable/create_record: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("airtable/create_record: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("airtable/create_record: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("airtable/create_record: Airtable API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var record struct {
		ID          string         `json:"id"`
		CreatedTime string         `json:"createdTime"`
		Fields      map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(respBody, &record); err != nil {
		return nil, fmt.Errorf("airtable/create_record: parsing response: %w", err)
	}

	return map[string]any{
		"id":           record.ID,
		"created_time": record.CreatedTime,
		"fields":       record.Fields,
	}, nil
}

