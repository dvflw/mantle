package connector

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 -- Mailchimp API requires MD5(lowercase(email)) as subscriber hash; not a security primitive
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MailchimpListMembersConnector lists members of a Mailchimp audience list.
type MailchimpListMembersConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *MailchimpListMembersConnector) apiURL(dc, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return fmt.Sprintf("https://%s.api.mailchimp.com/3.0%s", dc, path)
}

func (c *MailchimpListMembersConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	dc, apiKey, err := extractMailchimpCredential(params)
	if err != nil {
		return nil, fmt.Errorf("mailchimp/list_members: %w", err)
	}

	listID, _ := params["list_id"].(string)
	if listID == "" {
		return nil, fmt.Errorf("mailchimp/list_members: list_id is required")
	}

	endpoint := c.apiURL(dc, fmt.Sprintf("/lists/%s/members", listID))
	if count, ok := extractInt(params["count"]); ok && count > 0 {
		endpoint += fmt.Sprintf("?count=%d", count)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("mailchimp/list_members: creating request: %w", err)
	}
	req.SetBasicAuth("anystring", apiKey)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("mailchimp/list_members: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("mailchimp/list_members: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mailchimp/list_members: Mailchimp API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var result struct {
		Members    []any `json:"members"`
		TotalItems int   `json:"total_items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("mailchimp/list_members: parsing response: %w", err)
	}
	return map[string]any{
		"members":     result.Members,
		"count":       len(result.Members),
		"total_items": result.TotalItems,
	}, nil
}

// MailchimpAddMemberConnector adds or updates a subscriber in a Mailchimp audience list.
type MailchimpAddMemberConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *MailchimpAddMemberConnector) apiURL(dc, path string) string {
	if c.baseURL != "" {
		return c.baseURL + path
	}
	return fmt.Sprintf("https://%s.api.mailchimp.com/3.0%s", dc, path)
}

func (c *MailchimpAddMemberConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	dc, apiKey, err := extractMailchimpCredential(params)
	if err != nil {
		return nil, fmt.Errorf("mailchimp/add_member: %w", err)
	}

	listID, _ := params["list_id"].(string)
	if listID == "" {
		return nil, fmt.Errorf("mailchimp/add_member: list_id is required")
	}
	email, _ := params["email"].(string)
	if email == "" {
		return nil, fmt.Errorf("mailchimp/add_member: email is required")
	}

	status := "subscribed"
	if s, ok := params["status"].(string); ok && s != "" {
		status = s
	}

	member := map[string]any{
		"email_address": email,
		"status":        status,
	}
	if mergeFields, ok := params["merge_fields"].(map[string]any); ok {
		member["merge_fields"] = mergeFields
	}
	if tags, ok := params["tags"].([]any); ok && len(tags) > 0 {
		member["tags"] = tags
	}

	reqJSON, err := json.Marshal(member)
	if err != nil {
		return nil, fmt.Errorf("mailchimp/add_member: marshaling request: %w", err)
	}

	// codeql[go/weak-sensitive-data-hashing]
	hash := md5.Sum([]byte(strings.ToLower(email))) // #nosec G401 G501 -- Mailchimp API mandates MD5(lowercase(email)) as subscriber hash
	subscriberHash := hex.EncodeToString(hash[:])

	req, err := http.NewRequestWithContext(ctx, "PUT",
		c.apiURL(dc, fmt.Sprintf("/lists/%s/members/%s", listID, subscriberHash)),
		bytes.NewReader(reqJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("mailchimp/add_member: creating request: %w", err)
	}
	req.SetBasicAuth("anystring", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("mailchimp/add_member: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("mailchimp/add_member: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("mailchimp/add_member: Mailchimp API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "mailchimp/add_member")
}

func extractMailchimpCredential(params map[string]any) (dc, apiKey string, err error) {
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

	apiKey = cred["api_key"]
	if apiKey == "" {
		return "", "", fmt.Errorf("credential must contain an 'api_key' field")
	}

	// Data center is the suffix after the last '-' in the API key (e.g. "abc123-us1" → "us1").
	parts := strings.Split(apiKey, "-")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("credential api_key format invalid: expected '<key>-<dc>' (e.g. abc123-us1)")
	}
	dc = parts[len(parts)-1]
	return dc, apiKey, nil
}
