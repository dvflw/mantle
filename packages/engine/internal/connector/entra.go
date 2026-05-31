package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const entraBaseURL = "https://graph.microsoft.com/v1.0"

// EntraListUsersConnector lists users from Microsoft Entra ID (Azure AD).
type EntraListUsersConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *EntraListUsersConnector) apiURL(path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return entraBaseURL + path
}

func (c *EntraListUsersConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("entra/list_users: %w", err)
	}

	query := url.Values{}
	query.Set("$select", "id,displayName,mail,userPrincipalName,accountEnabled")
	if filter, ok := params["filter"].(string); ok && filter != "" {
		query.Set("$filter", filter)
	}
	if top, ok := extractInt(params["top"]); ok && top > 0 {
		query.Set("$top", fmt.Sprintf("%d", top))
	}

	endpoint := c.apiURL("/users?" + query.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("entra/list_users: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("entra/list_users: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("entra/list_users: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("entra/list_users: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var result struct {
		Value []any `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("entra/list_users: parsing response: %w", err)
	}

	return map[string]any{
		"users": result.Value,
		"count": len(result.Value),
	}, nil
}

// EntraCreateUserConnector creates a user in Microsoft Entra ID.
type EntraCreateUserConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *EntraCreateUserConnector) apiURL(path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return entraBaseURL + path
}

func (c *EntraCreateUserConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("entra/create_user: %w", err)
	}

	displayName, _ := params["display_name"].(string)
	if displayName == "" {
		return nil, fmt.Errorf("entra/create_user: display_name is required")
	}
	upn, _ := params["user_principal_name"].(string)
	if upn == "" {
		return nil, fmt.Errorf("entra/create_user: user_principal_name is required")
	}
	password, _ := params["password"].(string)
	if password == "" {
		return nil, fmt.Errorf("entra/create_user: password is required")
	}

	mailNickname, _ := params["mail_nickname"].(string)
	if mailNickname == "" {
		// Default to the part of UPN before @
		if idx := strings.Index(upn, "@"); idx > 0 {
			mailNickname = upn[:idx]
		} else {
			mailNickname = upn
		}
	}

	bodyMap := map[string]any{
		"displayName":       displayName,
		"userPrincipalName": upn,
		"mailNickname":      mailNickname,
		"accountEnabled":    true,
		"passwordProfile": map[string]any{
			"password":                      password,
			"forceChangePasswordNextSignIn": false,
		},
	}

	reqJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("entra/create_user: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL("/users"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("entra/create_user: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("entra/create_user: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("entra/create_user: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("entra/create_user: API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	return parseJSONBody(respBody, "entra/create_user")
}

// EntraAddGroupMemberConnector adds a user to an Entra ID (Azure AD) group.
type EntraAddGroupMemberConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *EntraAddGroupMemberConnector) apiURL(path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return entraBaseURL + path
}

func (c *EntraAddGroupMemberConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("entra/add_group_member: %w", err)
	}

	groupID, _ := params["group_id"].(string)
	if groupID == "" {
		return nil, fmt.Errorf("entra/add_group_member: group_id is required")
	}
	userID, _ := params["user_id"].(string)
	if userID == "" {
		return nil, fmt.Errorf("entra/add_group_member: user_id is required")
	}

	// The @odata.id value always points to the production Graph endpoint.
	refBody := map[string]any{
		"@odata.id": "https://graph.microsoft.com/v1.0/directoryObjects/" + userID,
	}
	reqJSON, err := json.Marshal(refBody)
	if err != nil {
		return nil, fmt.Errorf("entra/add_group_member: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/groups/%s/members/$ref", groupID)
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(path), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("entra/add_group_member: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("entra/add_group_member: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("entra/add_group_member: API returned %d", resp.StatusCode)
	}

	return map[string]any{"ok": true}, nil
}
