package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const slackBaseURL = "https://slack.com/api"

// SlackSendConnector sends messages to Slack channels via chat.postMessage.
type SlackSendConnector struct {
	Client  *http.Client
	baseURL string // override for testing; empty uses slackBaseURL
}

func (c *SlackSendConnector) slackURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = slackBaseURL
	}
	return base + path
}

func (c *SlackSendConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	// Extract credential.
	token, err := extractSlackToken(params)
	if err != nil {
		return nil, fmt.Errorf("slack/send: %w", err)
	}

	channel, _ := params["channel"].(string)
	if channel == "" {
		return nil, fmt.Errorf("slack/send: channel is required")
	}

	text, _ := params["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("slack/send: text is required")
	}

	reqBody := map[string]string{
		"channel": channel,
		"text":    text,
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("slack/send: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.slackURL("/chat.postMessage"), bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("slack/send: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack/send: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("slack/send: reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("slack/send: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var slackResp slackResponse
	if err := json.Unmarshal(body, &slackResp); err != nil {
		return nil, fmt.Errorf("slack/send: parsing response: %w", err)
	}

	if !slackResp.OK {
		return nil, fmt.Errorf("slack/send: Slack API error: %s", slackResp.Error)
	}

	return map[string]any{
		"ok":      slackResp.OK,
		"ts":      slackResp.TS,
		"channel": slackResp.Channel,
	}, nil
}

// SlackHistoryConnector reads channel history via conversations.history.
type SlackHistoryConnector struct {
	Client  *http.Client
	baseURL string // override for testing; empty uses slackBaseURL
}

func (c *SlackHistoryConnector) slackURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = slackBaseURL
	}
	return base + path
}

func (c *SlackHistoryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	// Extract credential.
	token, err := extractSlackToken(params)
	if err != nil {
		return nil, fmt.Errorf("slack/history: %w", err)
	}

	channel, _ := params["channel"].(string)
	if channel == "" {
		return nil, fmt.Errorf("slack/history: channel is required")
	}

	limit := 10
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	} else if l, ok := params["limit"].(int); ok && l > 0 {
		limit = l
	}

	url := fmt.Sprintf("%s/conversations.history?channel=%s&limit=%d", c.slackURL(""), channel, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("slack/history: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack/history: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("slack/history: reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("slack/history: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var slackResp slackHistoryResponse
	if err := json.Unmarshal(body, &slackResp); err != nil {
		return nil, fmt.Errorf("slack/history: parsing response: %w", err)
	}

	if !slackResp.OK {
		return nil, fmt.Errorf("slack/history: Slack API error: %s", slackResp.Error)
	}

	// Convert messages to []any for consistent output.
	messages := make([]any, len(slackResp.Messages))
	for i, msg := range slackResp.Messages {
		messages[i] = msg
	}

	return map[string]any{
		"ok":       slackResp.OK,
		"messages": messages,
	}, nil
}

// extractSlackToken pulls the Slack bot token from _credential and deletes the key.
func extractSlackToken(params map[string]any) (string, error) {
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

type slackResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	TS      string `json:"ts,omitempty"`
	Channel string `json:"channel,omitempty"`
}

type slackHistoryResponse struct {
	OK       bool             `json:"ok"`
	Error    string           `json:"error,omitempty"`
	Messages []map[string]any `json:"messages,omitempty"`
}
