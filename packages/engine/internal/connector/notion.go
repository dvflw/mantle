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

const (
	notionBaseURL = "https://api.notion.com/v1"
	notionVersion = "2022-06-28"
)

// NotionCreatePageConnector creates a page in a Notion database or under a parent page.
type NotionCreatePageConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *NotionCreatePageConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = notionBaseURL
	}
	return base + path
}

func (c *NotionCreatePageConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractNotionToken(params)
	if err != nil {
		return nil, fmt.Errorf("notion/create_page: %w", err)
	}

	parentDBID, _ := params["parent_database_id"].(string)
	parentPageID, _ := params["parent_page_id"].(string)
	if parentDBID == "" && parentPageID == "" {
		return nil, fmt.Errorf("notion/create_page: parent_database_id or parent_page_id is required")
	}

	var parent map[string]any
	if parentDBID != "" {
		parent = map[string]any{"database_id": parentDBID}
	} else {
		parent = map[string]any{"page_id": parentPageID}
	}

	// Merge caller-supplied properties, then add title shorthand if not already set.
	// title_key names the database column that holds the title (defaults to "title";
	// many databases use "Name" or another user-defined label).
	titleKey := "title"
	if k, ok := params["title_key"].(string); ok && k != "" {
		titleKey = k
	}

	properties := map[string]any{}
	if p, ok := params["properties"].(map[string]any); ok {
		for k, v := range p {
			properties[k] = v
		}
	}
	if title, ok := params["title"].(string); ok && title != "" {
		if _, exists := properties[titleKey]; !exists {
			properties[titleKey] = map[string]any{
				"title": []any{
					map[string]any{"text": map[string]any{"content": title}},
				},
			}
		}
	}

	body := map[string]any{
		"parent":     parent,
		"properties": properties,
	}
	if children, ok := params["children"].([]any); ok && len(children) > 0 {
		body["children"] = children
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("notion/create_page: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL("/pages"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("notion/create_page: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", notionVersion)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("notion/create_page: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("notion/create_page: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notion/create_page: Notion API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var page struct {
		ID             string `json:"id"`
		URL            string `json:"url"`
		CreatedTime    string `json:"created_time"`
		LastEditedTime string `json:"last_edited_time"`
	}
	if err := json.Unmarshal(respBody, &page); err != nil {
		return nil, fmt.Errorf("notion/create_page: parsing response: %w", err)
	}

	return map[string]any{
		"id":               page.ID,
		"url":              page.URL,
		"created_time":     page.CreatedTime,
		"last_edited_time": page.LastEditedTime,
	}, nil
}

// NotionQueryDatabaseConnector queries a Notion database.
type NotionQueryDatabaseConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *NotionQueryDatabaseConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = notionBaseURL
	}
	return base + path
}

func (c *NotionQueryDatabaseConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractNotionToken(params)
	if err != nil {
		return nil, fmt.Errorf("notion/query_database: %w", err)
	}

	databaseID, _ := params["database_id"].(string)
	if databaseID == "" {
		return nil, fmt.Errorf("notion/query_database: database_id is required")
	}

	body := map[string]any{}
	if filter, ok := params["filter"].(map[string]any); ok {
		body["filter"] = filter
	}
	if sorts, ok := params["sorts"].([]any); ok && len(sorts) > 0 {
		body["sorts"] = sorts
	}
	if pageSize, ok := extractInt(params["page_size"]); ok && pageSize > 0 {
		body["page_size"] = pageSize
	}
	if cursor, ok := params["start_cursor"].(string); ok && cursor != "" {
		body["start_cursor"] = cursor
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("notion/query_database: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/databases/%s/query", url.PathEscape(databaseID))
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("notion/query_database: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", notionVersion)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("notion/query_database: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("notion/query_database: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notion/query_database: Notion API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Results    []any  `json:"results"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("notion/query_database: parsing response: %w", err)
	}

	out := map[string]any{
		"results":  result.Results,
		"has_more": result.HasMore,
		"count":    len(result.Results),
	}
	if result.NextCursor != "" {
		out["next_cursor"] = result.NextCursor
	}
	return out, nil
}

// extractNotionToken pulls the Notion integration token from _credential.
// Accepts both map[string]string (engine-injected) and map[string]any
// (JSON/CEL-deserialised) credential shapes.
func extractNotionToken(params map[string]any) (string, error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var token string
	switch cred := raw.(type) {
	case map[string]string:
		token = cred["token"]
	case map[string]any:
		token, _ = cred["token"].(string)
	default:
		return "", fmt.Errorf("credential is required")
	}
	if token == "" {
		return "", fmt.Errorf("credential must contain a 'token' field")
	}
	return token, nil
}
