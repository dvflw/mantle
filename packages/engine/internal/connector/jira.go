package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// extractJiraCredential extracts domain, email, and api_token from _credential.
// Accepts both map[string]string and map[string]any credential shapes.
func extractJiraCredential(params map[string]any) (domain, email, token string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", "", fmt.Errorf("credential is required")
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
		return "", "", "", fmt.Errorf("credential is required")
	}

	domain = cred["domain"]
	if domain == "" {
		return "", "", "", fmt.Errorf("credential must contain a 'domain' field (e.g. mycompany.atlassian.net)")
	}
	email = cred["email"]
	if email == "" {
		return "", "", "", fmt.Errorf("credential must contain an 'email' field")
	}
	token = cred["api_token"]
	if token == "" {
		return "", "", "", fmt.Errorf("credential must contain an 'api_token' field")
	}
	return domain, email, token, nil
}

// JiraCreateIssueConnector creates an issue in Jira.
type JiraCreateIssueConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *JiraCreateIssueConnector) apiURL(domain, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return "https://" + domain + path
}

func (c *JiraCreateIssueConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	domain, email, token, err := extractJiraCredential(params)
	if err != nil {
		return nil, fmt.Errorf("jira/create_issue: %w", err)
	}

	projectKey, _ := params["project_key"].(string)
	if projectKey == "" {
		return nil, fmt.Errorf("jira/create_issue: project_key is required")
	}
	summary, _ := params["summary"].(string)
	if summary == "" {
		return nil, fmt.Errorf("jira/create_issue: summary is required")
	}

	issueType, _ := params["issue_type"].(string)
	if issueType == "" {
		issueType = "Task"
	}

	fields := map[string]any{
		"project":   map[string]any{"key": projectKey},
		"summary":   summary,
		"issuetype": map[string]any{"name": issueType},
	}

	if desc, ok := params["description"].(string); ok && desc != "" {
		fields["description"] = map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []any{
				map[string]any{
					"type":    "paragraph",
					"content": []any{map[string]any{"type": "text", "text": desc}},
				},
			},
		}
	}
	if priority, ok := params["priority"].(string); ok && priority != "" {
		fields["priority"] = map[string]any{"name": priority}
	}
	if assignee, ok := params["assignee"].(string); ok && assignee != "" {
		fields["assignee"] = map[string]any{"accountId": assignee}
	}

	bodyMap := map[string]any{"fields": fields}
	reqJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("jira/create_issue: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(domain, "/rest/api/3/issue"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("jira/create_issue: creating request: %w", err)
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira/create_issue: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("jira/create_issue: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira/create_issue: Jira API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	return parseJSONBody(respBody, "jira/create_issue")
}

// JiraSearchIssuesConnector searches for Jira issues using JQL.
type JiraSearchIssuesConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *JiraSearchIssuesConnector) apiURL(domain, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return "https://" + domain + path
}

func (c *JiraSearchIssuesConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	domain, email, token, err := extractJiraCredential(params)
	if err != nil {
		return nil, fmt.Errorf("jira/search_issues: %w", err)
	}

	jql, _ := params["jql"].(string)
	if jql == "" {
		return nil, fmt.Errorf("jira/search_issues: jql is required")
	}

	maxResults := 20
	if m, ok := extractInt(params["max_results"]); ok && m > 0 {
		maxResults = m
	}

	fields := []string{"summary", "status", "assignee", "priority", "issuetype", "created", "updated"}
	if f := toStringSlice(params["fields"]); len(f) > 0 {
		fields = f
	}

	bodyMap := map[string]any{
		"jql":        jql,
		"maxResults": maxResults,
		"fields":     fields,
	}
	reqJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("jira/search_issues: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(domain, "/rest/api/3/issue/search"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("jira/search_issues: creating request: %w", err)
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira/search_issues: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("jira/search_issues: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira/search_issues: Jira API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Issues []any `json:"issues"`
		Total  int   `json:"total"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("jira/search_issues: parsing response: %w", err)
	}

	return map[string]any{
		"issues": result.Issues,
		"total":  result.Total,
		"count":  len(result.Issues),
	}, nil
}
