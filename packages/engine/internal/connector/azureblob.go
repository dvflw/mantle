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

// extractAzureBlobCredential extracts Azure Blob Storage credentials from params.
// Credential shape: {account, container, sas_token}
func extractAzureBlobCredential(params map[string]any) (account, container, sasToken string, err error) {
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

	account = cred["account"]
	if account == "" {
		return "", "", "", fmt.Errorf("credential must contain an 'account' field")
	}
	container = cred["container"]
	if container == "" {
		return "", "", "", fmt.Errorf("credential must contain a 'container' field")
	}
	sasToken = cred["sas_token"]
	return account, container, sasToken, nil
}

// extractAzureFunctionCredential extracts Azure Function credentials from params.
// Credential shape: {function_url, function_key (optional)}
func extractAzureFunctionCredential(params map[string]any) (functionURL, functionKey string, err error) {
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

	functionURL = cred["function_url"]
	if functionURL == "" {
		return "", "", fmt.Errorf("credential must contain a 'function_url' field")
	}
	functionKey = cred["function_key"]
	return functionURL, functionKey, nil
}

// AzureBlobUploadConnector uploads a blob to Azure Blob Storage.
type AzureBlobUploadConnector struct {
	Client *http.Client
}

func (c *AzureBlobUploadConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	account, container, sasToken, err := extractAzureBlobCredential(params)
	if err != nil {
		return nil, fmt.Errorf("azure/blob_upload: %w", err)
	}

	blobName, _ := params["blob_name"].(string)
	if blobName == "" {
		return nil, fmt.Errorf("azure/blob_upload: blob_name is required")
	}
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("azure/blob_upload: content is required")
	}

	contentType := "application/octet-stream"
	if ct, ok := params["content_type"].(string); ok && ct != "" {
		contentType = ct
	}

	endpoint := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", account, container, blobName)
	if sasToken != "" {
		endpoint += "?" + sasToken
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader([]byte(content)))
	if err != nil {
		return nil, fmt.Errorf("azure/blob_upload: creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-ms-blob-type", "BlockBlob")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("azure/blob_upload: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("azure/blob_upload: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure/blob_upload: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return map[string]any{
		"ok":        true,
		"blob_name": blobName,
	}, nil
}

// AzureBlobDownloadConnector downloads a blob from Azure Blob Storage.
type AzureBlobDownloadConnector struct {
	Client *http.Client
}

func (c *AzureBlobDownloadConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	account, container, sasToken, err := extractAzureBlobCredential(params)
	if err != nil {
		return nil, fmt.Errorf("azure/blob_download: %w", err)
	}

	blobName, _ := params["blob_name"].(string)
	if blobName == "" {
		return nil, fmt.Errorf("azure/blob_download: blob_name is required")
	}

	endpoint := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", account, container, blobName)
	if sasToken != "" {
		endpoint += "?" + sasToken
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("azure/blob_download: creating request: %w", err)
	}

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("azure/blob_download: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("azure/blob_download: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure/blob_download: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return map[string]any{
		"content":      string(body),
		"content_type": resp.Header.Get("Content-Type"),
	}, nil
}

// AzureInvokeFunctionConnector invokes an Azure Function.
type AzureInvokeFunctionConnector struct {
	Client *http.Client
}

func (c *AzureInvokeFunctionConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	functionURL, functionKey, err := extractAzureFunctionCredential(params)
	if err != nil {
		return nil, fmt.Errorf("azure/invoke_function: %w", err)
	}

	method := "POST"
	if m, ok := params["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	endpoint := functionURL
	if functionKey != "" {
		if strings.Contains(endpoint, "?") {
			endpoint += "&code=" + url.QueryEscape(functionKey)
		} else {
			endpoint += "?code=" + url.QueryEscape(functionKey)
		}
	}

	var bodyReader io.Reader
	if bodyStr, ok := params["body"].(string); ok && bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("azure/invoke_function: creating request: %w", err)
	}

	if ct, ok := params["content_type"].(string); ok && ct != "" {
		req.Header.Set("Content-Type", ct)
	} else if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("azure/invoke_function: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("azure/invoke_function: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("azure/invoke_function: request failed with status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") && len(body) > 0 {
		var result map[string]any
		if jsonErr := json.Unmarshal(body, &result); jsonErr == nil {
			return result, nil
		}
	}

	return map[string]any{
		"body":   string(body),
		"status": resp.StatusCode,
	}, nil
}
