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

const asanaBaseURL = "https://app.asana.com/api/1.0"

// AsanaCreateTaskConnector creates a task in Asana.
type AsanaCreateTaskConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *AsanaCreateTaskConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = asanaBaseURL
	}
	return base + path
}

func (c *AsanaCreateTaskConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractAsanaToken(params)
	if err != nil {
		return nil, fmt.Errorf("asana/create_task: %w", err)
	}

	name, _ := params["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("asana/create_task: name is required")
	}

	workspace, _ := params["workspace"].(string)
	projects, _ := params["projects"].([]any)
	if workspace == "" && len(projects) == 0 {
		return nil, fmt.Errorf("asana/create_task: workspace or projects is required")
	}

	task := map[string]any{"name": name}
	if workspace != "" {
		task["workspace"] = workspace
	}
	if len(projects) > 0 {
		task["projects"] = projects
	}
	if notes, ok := params["notes"].(string); ok && notes != "" {
		task["notes"] = notes
	}
	if assignee, ok := params["assignee"].(string); ok && assignee != "" {
		task["assignee"] = assignee
	}
	if dueOn, ok := params["due_on"].(string); ok && dueOn != "" {
		task["due_on"] = dueOn
	}

	reqJSON, err := json.Marshal(map[string]any{"data": task})
	if err != nil {
		return nil, fmt.Errorf("asana/create_task: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL("/tasks"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("asana/create_task: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("asana/create_task: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("asana/create_task: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("asana/create_task: Asana API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Data struct {
			GID          string `json:"gid"`
			Name         string `json:"name"`
			PermalinkURL string `json:"permalink_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("asana/create_task: parsing response: %w", err)
	}

	return map[string]any{
		"gid":           result.Data.GID,
		"name":          result.Data.Name,
		"permalink_url": result.Data.PermalinkURL,
	}, nil
}

// AsanaSearchConnector searches tasks within an Asana workspace.
type AsanaSearchConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *AsanaSearchConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = asanaBaseURL
	}
	return base + path
}

func (c *AsanaSearchConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractAsanaToken(params)
	if err != nil {
		return nil, fmt.Errorf("asana/search: %w", err)
	}

	workspaceGID, _ := params["workspace"].(string)
	if workspaceGID == "" {
		return nil, fmt.Errorf("asana/search: workspace is required")
	}

	q := url.Values{}
	if text, ok := params["text"].(string); ok && text != "" {
		q.Set("text", text)
	}
	if assignee, ok := params["assignee"].(string); ok && assignee != "" {
		q.Set("assignee.any", assignee)
	}
	if completed, ok := params["completed"].(bool); ok {
		if completed {
			q.Set("completed", "true")
		} else {
			q.Set("completed", "false")
		}
	}
	if limit, ok := extractInt(params["limit"]); ok && limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}

	path := fmt.Sprintf("/workspaces/%s/tasks/search", url.PathEscape(workspaceGID))
	reqURL := c.apiURL(path)
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("asana/search: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("asana/search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("asana/search: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asana/search: Asana API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("asana/search: parsing response: %w", err)
	}

	return map[string]any{
		"tasks": result.Data,
		"count": len(result.Data),
	}, nil
}

func extractAsanaToken(params map[string]any) (string, error) {
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
