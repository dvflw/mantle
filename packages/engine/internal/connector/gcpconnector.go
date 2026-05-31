package connector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const gcpPubSubBaseURL = "https://pubsub.googleapis.com/v1"

// GCPPubSubPublishConnector publishes a message to a Google Cloud Pub/Sub topic.
type GCPPubSubPublishConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *GCPPubSubPublishConnector) getBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return gcpPubSubBaseURL
}

func (c *GCPPubSubPublishConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("gcp/publish: %w", err)
	}

	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		return nil, fmt.Errorf("gcp/publish: project_id is required")
	}
	topicID, _ := params["topic_id"].(string)
	if topicID == "" {
		return nil, fmt.Errorf("gcp/publish: topic_id is required")
	}
	message, _ := params["message"].(string)
	if message == "" {
		return nil, fmt.Errorf("gcp/publish: message is required")
	}

	attributes, _ := params["attributes"].(map[string]any)

	encoded := base64.StdEncoding.EncodeToString([]byte(message))
	msg := map[string]any{
		"data": encoded,
	}
	if len(attributes) > 0 {
		msg["attributes"] = attributes
	}

	payload, err := json.Marshal(map[string]any{
		"messages": []any{msg},
	})
	if err != nil {
		return nil, fmt.Errorf("gcp/publish: marshaling body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/projects/%s/topics/%s:publish",
		c.getBaseURL(), projectID, topicID)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gcp/publish: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("gcp/publish: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("gcp/publish: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gcp/publish: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "gcp/publish")
}

// GCPInvokeCloudRunConnector makes an authenticated HTTP request to a Cloud Run service.
type GCPInvokeCloudRunConnector struct {
	Client *http.Client
}

func (c *GCPInvokeCloudRunConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("gcp/invoke_cloud_run: %w", err)
	}

	serviceURL, _ := params["url"].(string)
	if serviceURL == "" {
		return nil, fmt.Errorf("gcp/invoke_cloud_run: url is required")
	}

	method := "POST"
	if m, ok := params["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	var bodyReader io.Reader
	if bodyStr, ok := params["body"].(string); ok && bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, serviceURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("gcp/invoke_cloud_run: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	if headers, ok := params["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("gcp/invoke_cloud_run: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("gcp/invoke_cloud_run: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gcp/invoke_cloud_run: request failed with status %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") && len(body) > 0 {
		result, parseErr := parseJSONBody(body, "gcp/invoke_cloud_run")
		if parseErr == nil {
			return result, nil
		}
	}

	return map[string]any{
		"body":   string(body),
		"status": resp.StatusCode,
	}, nil
}
