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

// OktaListUsersConnector lists users from an Okta organization.
type OktaListUsersConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *OktaListUsersConnector) apiURL(domain, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return "https://" + domain + path
}

func (c *OktaListUsersConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	domain, token, err := extractOktaCredential(params)
	if err != nil {
		return nil, fmt.Errorf("okta/list_users: %w", err)
	}

	query := url.Values{}
	if q, ok := params["q"].(string); ok && q != "" {
		query.Set("q", q)
	}
	if filter, ok := params["filter"].(string); ok && filter != "" {
		query.Set("filter", filter)
	}
	if limit, ok := extractInt(params["limit"]); ok && limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}

	endpoint := c.apiURL(domain, "/api/v1/users")
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("okta/list_users: creating request: %w", err)
	}
	req.Header.Set("Authorization", "SSWS "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("okta/list_users: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("okta/list_users: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("okta/list_users: Okta API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var users []any
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("okta/list_users: parsing response: %w", err)
	}
	return map[string]any{"users": users, "count": len(users)}, nil
}

// OktaCreateUserConnector creates a new user in an Okta organization.
type OktaCreateUserConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *OktaCreateUserConnector) apiURL(domain, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return "https://" + domain + path
}

func (c *OktaCreateUserConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	domain, token, err := extractOktaCredential(params)
	if err != nil {
		return nil, fmt.Errorf("okta/create_user: %w", err)
	}

	profile, _ := params["profile"].(map[string]any)
	if profile == nil {
		return nil, fmt.Errorf("okta/create_user: profile is required")
	}
	if _, ok := profile["login"]; !ok {
		return nil, fmt.Errorf("okta/create_user: profile.login is required")
	}
	if _, ok := profile["email"]; !ok {
		return nil, fmt.Errorf("okta/create_user: profile.email is required")
	}

	body := map[string]any{"profile": profile}
	if credentials, ok := params["credentials"].(map[string]any); ok {
		body["credentials"] = credentials
	}
	if groupIDs, ok := params["group_ids"].([]any); ok && len(groupIDs) > 0 {
		body["groupIds"] = groupIDs
	}

	activate := true
	if a, ok := params["activate"].(bool); ok {
		activate = a
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("okta/create_user: marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf("%s?activate=%v", c.apiURL(domain, "/api/v1/users"), activate)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("okta/create_user: creating request: %w", err)
	}
	req.Header.Set("Authorization", "SSWS "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("okta/create_user: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("okta/create_user: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("okta/create_user: Okta API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	return parseJSONBody(respBody, "okta/create_user")
}

func extractOktaCredential(params map[string]any) (domain, token string, err error) {
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

	domain = cred["domain"]
	if domain == "" {
		return "", "", fmt.Errorf("credential must contain a 'domain' field (e.g. dev-xxx.okta.com)")
	}
	token = cred["token"]
	if token == "" {
		return "", "", fmt.Errorf("credential must contain a 'token' field")
	}
	return domain, token, nil
}
