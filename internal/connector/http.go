package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPConnector executes HTTP requests.
type HTTPConnector struct {
	Client *http.Client // if nil, a default client with 30s timeout is used
}

func (c *HTTPConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	method, _ := params["method"].(string)
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	url, _ := params["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("http/request: url is required")
	}

	var bodyReader io.Reader
	if body, ok := params["body"]; ok {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("http/request: marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http/request: creating request: %w", err)
	}

	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Apply credential-based authentication before custom headers,
	// so user-specified headers can override if needed.
	if cred, ok := params["_credential"].(map[string]string); ok {
		switch {
		case cred["token"] != "":
			req.Header.Set("Authorization", "Bearer "+cred["token"])
		case cred["username"] != "" && cred["password"] != "":
			req.SetBasicAuth(cred["username"], cred["password"])
		case cred["api_key"] != "":
			req.Header.Set("Authorization", "Bearer "+cred["api_key"])
		case cred["key"] != "":
			req.Header.Set("Authorization", "Bearer "+cred["key"])
		}
		delete(params, "_credential")
	}

	// Apply custom headers.
	if headers, ok := params["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	client := c.Client
	if client == nil {
		timeout := 30 * time.Second
		if t, ok := params["timeout"].(string); ok {
			if d, err := time.ParseDuration(t); err == nil {
				timeout = d
			}
		}
		client = &http.Client{Timeout: timeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http/request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http/request: reading response: %w", err)
	}

	output := map[string]any{
		"status":  int64(resp.StatusCode),
		"headers": headerMap(resp.Header),
		"body":    string(respBody),
	}

	// Try to parse JSON response body into structured data.
	var parsed any
	if json.Unmarshal(respBody, &parsed) == nil {
		output["json"] = parsed
	}

	if resp.StatusCode >= 400 {
		return output, fmt.Errorf("http/request: %s %s returned %d", method, url, resp.StatusCode)
	}

	return output, nil
}

func headerMap(h http.Header) map[string]any {
	result := make(map[string]any, len(h))
	for k, v := range h {
		if len(v) == 1 {
			result[strings.ToLower(k)] = v[0]
		} else {
			result[strings.ToLower(k)] = v
		}
	}
	return result
}
