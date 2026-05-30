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

// ElasticsearchSearchConnector executes a search query against an Elasticsearch index.
type ElasticsearchSearchConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *ElasticsearchSearchConnector) apiURL(base, path string) string {
	if base == "" {
		base = c.baseURL
	}
	return base + path
}

func (c *ElasticsearchSearchConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	baseURL, auth, err := extractElasticsearchCredential(params)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/search: %w", err)
	}
	if c.baseURL != "" {
		baseURL = c.baseURL
	}

	index, _ := params["index"].(string)
	if index == "" {
		return nil, fmt.Errorf("elasticsearch/search: index is required")
	}

	body := map[string]any{}
	if query, ok := params["query"].(map[string]any); ok {
		body["query"] = query
	}
	if size, ok := extractInt(params["size"]); ok && size > 0 {
		body["size"] = size
	}
	if from, ok := extractInt(params["from"]); ok && from >= 0 {
		body["from"] = from
	}
	if source, ok := params["_source"].([]any); ok {
		body["_source"] = source
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/search: marshaling request: %w", err)
	}

	path := fmt.Sprintf("/%s/_search", url.PathEscape(index))
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+path, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/search: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	auth(req)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/search: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elasticsearch/search: Elasticsearch returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Took int  `json:"took"`
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []any `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("elasticsearch/search: parsing response: %w", err)
	}

	return map[string]any{
		"hits":  result.Hits.Hits,
		"total": result.Hits.Total.Value,
		"took":  result.Took,
		"count": len(result.Hits.Hits),
	}, nil
}

// ElasticsearchIndexConnector indexes a document into Elasticsearch.
type ElasticsearchIndexConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *ElasticsearchIndexConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	baseURL, auth, err := extractElasticsearchCredential(params)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/index: %w", err)
	}
	if c.baseURL != "" {
		baseURL = c.baseURL
	}

	index, _ := params["index"].(string)
	if index == "" {
		return nil, fmt.Errorf("elasticsearch/index: index is required")
	}
	document, _ := params["document"].(map[string]any)
	if document == nil {
		return nil, fmt.Errorf("elasticsearch/index: document is required")
	}

	reqJSON, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/index: marshaling document: %w", err)
	}

	method := "POST"
	path := fmt.Sprintf("/%s/_doc", url.PathEscape(index))
	if docID, ok := params["id"].(string); ok && docID != "" {
		method = "PUT"
		path = fmt.Sprintf("/%s/_doc/%s", url.PathEscape(index), url.PathEscape(docID))
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/index: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	auth(req)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/index: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("elasticsearch/index: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("elasticsearch/index: Elasticsearch returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		ID     string `json:"_id"`
		Index  string `json:"_index"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("elasticsearch/index: parsing response: %w", err)
	}

	return map[string]any{
		"id":     result.ID,
		"index":  result.Index,
		"result": result.Result,
	}, nil
}

// extractElasticsearchCredential extracts the base URL and auth setter from _credential.
// Credential fields: url (required), username+password (basic auth) or api_key (ApiKey auth).
func extractElasticsearchCredential(params map[string]any) (baseURL string, setAuth func(*http.Request), err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", nil, fmt.Errorf("credential is required")
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
		return "", nil, fmt.Errorf("credential is required")
	}

	baseURL = cred["url"]
	if baseURL == "" {
		return "", nil, fmt.Errorf("credential must contain a 'url' field")
	}

	if apiKey := cred["api_key"]; apiKey != "" {
		setAuth = func(r *http.Request) { r.Header.Set("Authorization", "ApiKey "+apiKey) }
	} else if username := cred["username"]; username != "" {
		setAuth = func(r *http.Request) { r.SetBasicAuth(username, cred["password"]) }
	} else {
		setAuth = func(r *http.Request) {}
	}

	return baseURL, setAuth, nil
}
