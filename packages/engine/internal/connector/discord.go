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

const discordBaseURL = "https://discord.com/api/v10"

// DiscordSendConnector sends a text message to a Discord channel.
type DiscordSendConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *DiscordSendConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = discordBaseURL
	}
	return base + path
}

func (c *DiscordSendConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("discord/send: %w", err)
	}

	channelID, _ := params["channel_id"].(string)
	if channelID == "" {
		return nil, fmt.Errorf("discord/send: channel_id is required")
	}
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("discord/send: content is required")
	}

	body := map[string]any{"content": content}
	if tts, ok := params["tts"].(bool); ok {
		body["tts"] = tts
	}

	return discordPostMessage(ctx, c.apiURL(fmt.Sprintf("/channels/%s/messages", url.PathEscape(channelID))), token, body, "discord/send")
}

// DiscordEmbedConnector sends an embed message to a Discord channel.
type DiscordEmbedConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *DiscordEmbedConnector) apiURL(path string) string {
	base := c.baseURL
	if base == "" {
		base = discordBaseURL
	}
	return base + path
}

func (c *DiscordEmbedConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("discord/embed: %w", err)
	}

	channelID, _ := params["channel_id"].(string)
	if channelID == "" {
		return nil, fmt.Errorf("discord/embed: channel_id is required")
	}

	embed := map[string]any{}
	if title, ok := params["title"].(string); ok && title != "" {
		embed["title"] = title
	}
	if description, ok := params["description"].(string); ok && description != "" {
		embed["description"] = description
	}
	if color, ok := extractInt(params["color"]); ok {
		embed["color"] = color
	}
	if url_, ok := params["url"].(string); ok && url_ != "" {
		embed["url"] = url_
	}
	if fields, ok := params["fields"].([]any); ok && len(fields) > 0 {
		embed["fields"] = fields
	}
	if footer, ok := params["footer"].(map[string]any); ok {
		embed["footer"] = footer
	}

	body := map[string]any{"embeds": []any{embed}}
	if content, ok := params["content"].(string); ok && content != "" {
		body["content"] = content
	}

	return discordPostMessage(ctx, c.apiURL(fmt.Sprintf("/channels/%s/messages", url.PathEscape(channelID))), token, body, "discord/embed")
}

func discordPostMessage(ctx context.Context, endpoint, token string, body map[string]any, action string) (map[string]any, error) {
	reqJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshaling request: %w", action, err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("%s: creating request: %w", action, err)
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(nil).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", action, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: Discord API returned %d: %s", action, resp.StatusCode, truncate(string(respBody), 500))
	}

	var msg struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(respBody, &msg); err != nil {
		return nil, fmt.Errorf("%s: parsing response: %w", action, err)
	}

	return map[string]any{
		"id":         msg.ID,
		"channel_id": msg.ChannelID,
		"timestamp":  msg.Timestamp,
	}, nil
}

