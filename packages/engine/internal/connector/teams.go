package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// extractTeamsCredential extracts the webhook_url from _credential.
// Accepts both map[string]string and map[string]any credential shapes.
func extractTeamsCredential(params map[string]any) (webhookURL string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", fmt.Errorf("credential is required")
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
		return "", fmt.Errorf("credential is required")
	}

	webhookURL = cred["webhook_url"]
	if webhookURL == "" {
		return "", fmt.Errorf("credential must contain a 'webhook_url' field")
	}
	return webhookURL, nil
}

// TeamsSendMessageConnector sends a plain text message to a Teams incoming webhook.
type TeamsSendMessageConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *TeamsSendMessageConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	webhookURL, err := extractTeamsCredential(params)
	if err != nil {
		return nil, fmt.Errorf("teams/send_message: %w", err)
	}

	text, _ := params["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("teams/send_message: text is required")
	}

	body := map[string]any{"text": text}
	if title, ok := params["title"].(string); ok && title != "" {
		body["title"] = title
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("teams/send_message: marshaling request: %w", err)
	}

	// Use baseURL for testing; otherwise use the webhook URL from credential.
	endpoint := webhookURL
	if c.baseURL != "" {
		endpoint = c.baseURL
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("teams/send_message: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("teams/send_message: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("teams/send_message: Teams returned %d", resp.StatusCode)
	}

	return map[string]any{"ok": true}, nil
}

// TeamsSendAdaptiveCardConnector sends an adaptive card to a Teams incoming webhook.
type TeamsSendAdaptiveCardConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *TeamsSendAdaptiveCardConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	webhookURL, err := extractTeamsCredential(params)
	if err != nil {
		return nil, fmt.Errorf("teams/send_adaptive_card: %w", err)
	}

	card, ok := params["card"].(map[string]any)
	if !ok || card == nil {
		return nil, fmt.Errorf("teams/send_adaptive_card: card is required")
	}

	body := map[string]any{
		"type": "message",
		"attachments": []any{
			map[string]any{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content":     card,
			},
		},
	}

	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("teams/send_adaptive_card: marshaling request: %w", err)
	}

	// Use baseURL for testing; otherwise use the webhook URL from credential.
	endpoint := webhookURL
	if c.baseURL != "" {
		endpoint = c.baseURL
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("teams/send_adaptive_card: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("teams/send_adaptive_card: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("teams/send_adaptive_card: Teams returned %d", resp.StatusCode)
	}

	return map[string]any{"ok": true}, nil
}
