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

const graphBaseURL = "https://graph.microsoft.com/v1.0"

// OneDriveUploadConnector uploads a file to OneDrive via the Microsoft Graph API.
type OneDriveUploadConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *OneDriveUploadConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = graphBaseURL
	}
	return base + path
}

func (c *OneDriveUploadConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("onedrive/upload: %w", err)
	}

	filePath, _ := params["path"].(string)
	if filePath == "" {
		return nil, fmt.Errorf("onedrive/upload: path is required")
	}
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("onedrive/upload: content is required")
	}

	// filePath must NOT be PathEscaped here: Graph path addressing uses
	// literal "/" separators inside the /root:/...:/content template, so
	// escaping them produces %2F which resolves to a file literally named
	// "documents%2Fhello.txt" instead of hello.txt inside the documents folder.
	var endpoint string
	if driveID, ok := params["drive_id"].(string); ok && driveID != "" {
		endpoint = c.apiURL(fmt.Sprintf("/drives/%s/root:/%s:/content",
			url.PathEscape(driveID), filePath))
	} else {
		endpoint = c.apiURL(fmt.Sprintf("/me/drive/root:/%s:/content", filePath))
	}

	contentType := "application/octet-stream"
	if ct, ok := params["content_type"].(string); ok && ct != "" {
		contentType = ct
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader([]byte(content)))
	if err != nil {
		return nil, fmt.Errorf("onedrive/upload: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("onedrive/upload: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("onedrive/upload: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("onedrive/upload: Graph API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "onedrive/upload")
}

// SharePointListItemsConnector lists items from a SharePoint list via the Microsoft Graph API.
type SharePointListItemsConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *SharePointListItemsConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = graphBaseURL
	}
	return base + path
}

func (c *SharePointListItemsConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("sharepoint/list_items: %w", err)
	}

	siteID, _ := params["site_id"].(string)
	if siteID == "" {
		return nil, fmt.Errorf("sharepoint/list_items: site_id is required")
	}
	listID, _ := params["list_id"].(string)
	if listID == "" {
		return nil, fmt.Errorf("sharepoint/list_items: list_id is required")
	}

	endpoint := c.apiURL(fmt.Sprintf("/sites/%s/lists/%s/items",
		url.PathEscape(siteID), url.PathEscape(listID)))

	query := url.Values{}
	query.Set("expand", "fields")
	if top, ok := extractInt(params["top"]); ok && top > 0 {
		query.Set("$top", fmt.Sprintf("%d", top))
	}
	if filter, ok := params["filter"].(string); ok && filter != "" {
		query.Set("$filter", filter)
	}
	endpoint += "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("sharepoint/list_items: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("sharepoint/list_items: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("sharepoint/list_items: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sharepoint/list_items: Graph API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var result struct {
		Value []any `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sharepoint/list_items: parsing response: %w", err)
	}
	return map[string]any{
		"items": result.Value,
		"count": len(result.Value),
	}, nil
}
