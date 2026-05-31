package connector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	googleDriveUploadURL = "https://www.googleapis.com/upload/drive/v3/files"
	googleDriveFilesURL  = "https://www.googleapis.com/drive/v3/files"
)

// GoogleDriveUploadConnector uploads a file to Google Drive via multipart upload.
type GoogleDriveUploadConnector struct {
	Client    *http.Client
	uploadURL string
}

func (c *GoogleDriveUploadConnector) getUploadURL() string {
	if c.uploadURL != "" {
		return c.uploadURL
	}
	return googleDriveUploadURL
}

func (c *GoogleDriveUploadConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("drive/upload: %w", err)
	}

	name, _ := params["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("drive/upload: name is required")
	}
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("drive/upload: content is required")
	}

	mimeType := "application/octet-stream"
	if mt, ok := params["mime_type"].(string); ok && mt != "" {
		mimeType = mt
	}
	parentID, _ := params["parent_id"].(string)

	// Build metadata JSON.
	var metaBody string
	if parentID != "" {
		metaBody = fmt.Sprintf(`{"name":%q,"parents":[%q]}`, name, parentID)
	} else {
		metaBody = fmt.Sprintf(`{"name":%q}`, name)
	}

	boundary := "mantle_multipart_boundary"
	var buf bytes.Buffer
	// Part 1: metadata
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: application/json\r\n\r\n")
	buf.WriteString(metaBody + "\r\n")
	// Part 2: content
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: " + mimeType + "\r\n\r\n")
	buf.WriteString(content + "\r\n")
	buf.WriteString("--" + boundary + "--\r\n")

	endpoint := c.getUploadURL() + "?uploadType=multipart"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("drive/upload: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("drive/upload: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("drive/upload: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("drive/upload: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "drive/upload")
}

// GoogleDriveListFilesConnector lists files in Google Drive.
type GoogleDriveListFilesConnector struct {
	Client   *http.Client
	filesURL string
}

func (c *GoogleDriveListFilesConnector) getFilesURL() string {
	if c.filesURL != "" {
		return c.filesURL
	}
	return googleDriveFilesURL
}

func (c *GoogleDriveListFilesConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("drive/list_files: %w", err)
	}

	pageSize := 20
	if ps, ok := extractInt(params["page_size"]); ok && ps > 0 {
		pageSize = ps
	}

	query, _ := params["query"].(string)
	folderID, _ := params["folder_id"].(string)
	if folderID != "" {
		prefix := fmt.Sprintf("'%s' in parents", folderID)
		if query != "" {
			query = prefix + " and " + query
		} else {
			query = prefix
		}
	}

	q := url.Values{}
	q.Set("fields", "files(id,name,mimeType,size,modifiedTime)")
	q.Set("pageSize", fmt.Sprintf("%d", pageSize))
	if query != "" {
		q.Set("q", query)
	}

	endpoint := c.getFilesURL() + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("drive/list_files: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("drive/list_files: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("drive/list_files: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("drive/list_files: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	result, err := parseJSONBody(body, "drive/list_files")
	if err != nil {
		return nil, err
	}

	files, _ := result["files"].([]any)
	return map[string]any{
		"files": files,
		"count": len(files),
	}, nil
}
