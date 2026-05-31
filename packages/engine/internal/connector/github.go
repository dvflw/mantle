package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const githubBaseURL = "https://api.github.com"

// GitHubCreateIssueConnector creates an issue in a GitHub repository.
type GitHubCreateIssueConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *GitHubCreateIssueConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = githubBaseURL
	}
	return base + path
}

func (c *GitHubCreateIssueConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractGitHubToken(params)
	if err != nil {
		return nil, fmt.Errorf("github/create_issue: %w", err)
	}

	owner, _ := params["owner"].(string)
	if owner == "" {
		return nil, fmt.Errorf("github/create_issue: owner is required")
	}
	repo, _ := params["repo"].(string)
	if repo == "" {
		return nil, fmt.Errorf("github/create_issue: repo is required")
	}
	title, _ := params["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("github/create_issue: title is required")
	}

	body := map[string]any{"title": title}
	if b, ok := params["body"].(string); ok && b != "" {
		body["body"] = b
	}
	if labels := toStringSlice(params["labels"]); len(labels) > 0 {
		body["labels"] = labels
	}
	if assignees := toStringSlice(params["assignees"]); len(assignees) > 0 {
		body["assignees"] = assignees
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("github/create_issue: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/repos/%s/%s/issues", owner, repo)
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("github/create_issue: creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("github/create_issue: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("github/create_issue: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("github/create_issue: GitHub API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var issue struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		NodeID  string `json:"node_id"`
		State   string `json:"state"`
		Title   string `json:"title"`
	}
	if err := json.Unmarshal(respBody, &issue); err != nil {
		return nil, fmt.Errorf("github/create_issue: parsing response: %w", err)
	}

	return map[string]any{
		"number":  issue.Number,
		"url":     issue.HTMLURL,
		"node_id": issue.NodeID,
		"state":   issue.State,
		"title":   issue.Title,
	}, nil
}

// GitHubDispatchConnector triggers a repository_dispatch event on a GitHub repository.
type GitHubDispatchConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *GitHubDispatchConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = githubBaseURL
	}
	return base + path
}

func (c *GitHubDispatchConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractGitHubToken(params)
	if err != nil {
		return nil, fmt.Errorf("github/dispatch: %w", err)
	}

	owner, _ := params["owner"].(string)
	if owner == "" {
		return nil, fmt.Errorf("github/dispatch: owner is required")
	}
	repo, _ := params["repo"].(string)
	if repo == "" {
		return nil, fmt.Errorf("github/dispatch: repo is required")
	}
	eventType, _ := params["event_type"].(string)
	if eventType == "" {
		return nil, fmt.Errorf("github/dispatch: event_type is required")
	}

	body := map[string]any{"event_type": eventType}
	if payload, ok := params["client_payload"].(map[string]any); ok {
		body["client_payload"] = payload
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("github/dispatch: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/repos/%s/%s/dispatches", owner, repo)
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("github/dispatch: creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("github/dispatch: %w", err)
	}
	defer resp.Body.Close()

	// GitHub returns 204 No Content on success.
	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("github/dispatch: GitHub API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	return map[string]any{"ok": true}, nil
}

// GitHubDispatchWorkflowConnector triggers a GitHub Actions workflow_dispatch event.
type GitHubDispatchWorkflowConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *GitHubDispatchWorkflowConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = githubBaseURL
	}
	return base + path
}

func (c *GitHubDispatchWorkflowConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("github/dispatch_workflow: %w", err)
	}

	owner, _ := params["owner"].(string)
	if owner == "" {
		return nil, fmt.Errorf("github/dispatch_workflow: owner is required")
	}
	repo, _ := params["repo"].(string)
	if repo == "" {
		return nil, fmt.Errorf("github/dispatch_workflow: repo is required")
	}
	workflowID, _ := params["workflow_id"].(string)
	if workflowID == "" {
		return nil, fmt.Errorf("github/dispatch_workflow: workflow_id is required")
	}
	ref, _ := params["ref"].(string)
	if ref == "" {
		return nil, fmt.Errorf("github/dispatch_workflow: ref is required")
	}

	body := map[string]any{"ref": ref}
	if inputs, ok := params["inputs"].(map[string]any); ok && len(inputs) > 0 {
		body["inputs"] = inputs
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("github/dispatch_workflow: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, url.PathEscape(workflowID))
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("github/dispatch_workflow: creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("github/dispatch_workflow: %w", err)
	}
	defer resp.Body.Close()

	// GitHub returns 204 No Content on success.
	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("github/dispatch_workflow: GitHub API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	return map[string]any{"ok": true}, nil
}

// extractGitHubToken pulls the GitHub token from _credential.
func extractGitHubToken(params map[string]any) (string, error) {
	cred, ok := params["_credential"].(map[string]string)
	if !ok {
		return "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	token := cred["token"]
	if token == "" {
		return "", fmt.Errorf("credential must contain a 'token' field")
	}
	return token, nil
}

// httpClient returns the provided client or a default with a 30s timeout.
func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// toStringSlice converts a []any or []string param value to []string.
func toStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
