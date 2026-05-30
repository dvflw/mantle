package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const linearGraphQLURL = "https://api.linear.app/graphql"

// LinearCreateIssueConnector creates an issue in a Linear team via the GraphQL API.
type LinearCreateIssueConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *LinearCreateIssueConnector) gqlURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return linearGraphQLURL
}

func (c *LinearCreateIssueConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractLinearToken(params)
	if err != nil {
		return nil, fmt.Errorf("linear/create_issue: %w", err)
	}

	teamID, _ := params["team_id"].(string)
	if teamID == "" {
		return nil, fmt.Errorf("linear/create_issue: team_id is required")
	}
	title, _ := params["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("linear/create_issue: title is required")
	}

	input := map[string]any{
		"teamId": teamID,
		"title":  title,
	}
	if desc, ok := params["description"].(string); ok && desc != "" {
		input["description"] = desc
	}
	if assigneeID, ok := params["assignee_id"].(string); ok && assigneeID != "" {
		input["assigneeId"] = assigneeID
	}
	if projectID, ok := params["project_id"].(string); ok && projectID != "" {
		input["projectId"] = projectID
	}
	if priority, ok := extractIntParam(params["priority"]); ok {
		input["priority"] = priority
	}
	if labelIDs := toStringSlice(params["label_ids"]); len(labelIDs) > 0 {
		input["labelIds"] = labelIDs
	}

	const mutation = `
mutation IssueCreate($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {
      id
      identifier
      title
      url
    }
  }
}`

	result, err := linearGQL(ctx, httpClient(c.Client), c.gqlURL(), token, mutation, map[string]any{"input": input})
	if err != nil {
		return nil, fmt.Errorf("linear/create_issue: %w", err)
	}

	created, ok := result["issueCreate"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("linear/create_issue: unexpected response shape")
	}
	if success, _ := created["success"].(bool); !success {
		return nil, fmt.Errorf("linear/create_issue: API returned success=false")
	}
	issue, ok := created["issue"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("linear/create_issue: missing issue in response")
	}

	return map[string]any{
		"id":         issue["id"],
		"identifier": issue["identifier"],
		"title":      issue["title"],
		"url":        issue["url"],
	}, nil
}

// LinearSearchConnector searches issues in Linear via the GraphQL API.
type LinearSearchConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *LinearSearchConnector) gqlURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return linearGraphQLURL
}

func (c *LinearSearchConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractLinearToken(params)
	if err != nil {
		return nil, fmt.Errorf("linear/search: %w", err)
	}

	limit := 25
	if l, ok := extractIntParam(params["limit"]); ok && l > 0 {
		limit = l
	}

	filter := map[string]any{}
	if query, ok := params["query"].(string); ok && query != "" {
		filter["title"] = map[string]any{"containsIgnoreCase": query}
	}
	if teamID, ok := params["team_id"].(string); ok && teamID != "" {
		filter["team"] = map[string]any{"id": map[string]any{"eq": teamID}}
	}
	if assigneeID, ok := params["assignee_id"].(string); ok && assigneeID != "" {
		filter["assignee"] = map[string]any{"id": map[string]any{"eq": assigneeID}}
	}
	if state, ok := params["state"].(string); ok && state != "" {
		filter["state"] = map[string]any{"name": map[string]any{"eq": state}}
	}

	const query = `
query Issues($filter: IssueFilter, $first: Int) {
  issues(filter: $filter, first: $first) {
    nodes {
      id
      identifier
      title
      url
      priority
      state { name }
      assignee { name email }
    }
  }
}`

	vars := map[string]any{"first": limit}
	if len(filter) > 0 {
		vars["filter"] = filter
	}

	result, err := linearGQL(ctx, httpClient(c.Client), c.gqlURL(), token, query, vars)
	if err != nil {
		return nil, fmt.Errorf("linear/search: %w", err)
	}

	issuesData, ok := result["issues"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("linear/search: unexpected response shape")
	}
	nodes, _ := issuesData["nodes"].([]any)

	return map[string]any{
		"issues": nodes,
		"count":  len(nodes),
	}, nil
}

// linearGQL executes a GraphQL request against the Linear API.
func linearGQL(ctx context.Context, client *http.Client, url, token, query string, variables map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Linear API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var gqlResp struct {
		Data   map[string]any   `json:"data"`
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		msg, _ := gqlResp.Errors[0]["message"].(string)
		return nil, fmt.Errorf("Linear GraphQL error: %s", msg)
	}

	return gqlResp.Data, nil
}

// extractLinearToken pulls the Linear API key from _credential.
func extractLinearToken(params map[string]any) (string, error) {
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

// extractIntParam extracts an integer from float64 (JSON default) or int.
func extractIntParam(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}
