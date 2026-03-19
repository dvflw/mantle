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

// AIConnector calls OpenAI-compatible chat completion APIs.
type AIConnector struct {
	Client *http.Client
}

func (c *AIConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	model, _ := params["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("ai/completion: model is required")
	}

	// Build messages: use _messages passthrough if provided, otherwise build from prompt/system_prompt.
	var messages any
	if rawMessages, ok := params["_messages"]; ok {
		messages = rawMessages
	} else {
		prompt, _ := params["prompt"].(string)
		if prompt == "" {
			return nil, fmt.Errorf("ai/completion: prompt is required")
		}
		var msgList []map[string]string
		if systemPrompt, _ := params["system_prompt"].(string); systemPrompt != "" {
			msgList = append(msgList, map[string]string{"role": "system", "content": systemPrompt})
		}
		msgList = append(msgList, map[string]string{"role": "user", "content": prompt})
		messages = msgList
	}

	// Build request body.
	reqBody := map[string]any{
		"model":    model,
		"messages": messages,
	}

	// Include tools for function calling.
	if tools, ok := params["_tools"]; ok {
		reqBody["tools"] = tools
	}

	// Structured output via response_format.
	if outputSchema, ok := params["output_schema"]; ok {
		reqBody["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "response",
				"strict": true,
				"schema": outputSchema,
			},
		}
	}

	// Determine API endpoint.
	baseURL := "https://api.openai.com/v1"
	if u, ok := params["base_url"].(string); ok && u != "" {
		baseURL = u
	}
	endpoint := baseURL + "/chat/completions"

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ai/completion: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("ai/completion: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Apply credential-based auth (same pattern as HTTP connector).
	if cred, ok := params["_credential"].(map[string]string); ok {
		switch {
		case cred["api_key"] != "":
			req.Header.Set("Authorization", "Bearer "+cred["api_key"])
		case cred["token"] != "":
			req.Header.Set("Authorization", "Bearer "+cred["token"])
		case cred["key"] != "":
			req.Header.Set("Authorization", "Bearer "+cred["key"])
		}
		if orgID := cred["org_id"]; orgID != "" {
			req.Header.Set("OpenAI-Organization", orgID)
		}
		delete(params, "_credential")
	}

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai/completion: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ai/completion: reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ai/completion: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	// Parse OpenAI response.
	var apiResp chatCompletionResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("ai/completion: parsing response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("ai/completion: no choices returned")
	}

	choice := apiResp.Choices[0]

	output := map[string]any{
		"model": apiResp.Model,
		"usage": map[string]any{
			"prompt_tokens":     apiResp.Usage.PromptTokens,
			"completion_tokens": apiResp.Usage.CompletionTokens,
			"total_tokens":      apiResp.Usage.TotalTokens,
		},
	}

	// If the model returned tool calls, surface them instead of text.
	if len(choice.Message.ToolCalls) > 0 {
		output["tool_calls"] = choice.Message.ToolCalls
		output["finish_reason"] = "tool_calls"
	} else {
		text := choice.Message.Content
		output["text"] = text
		output["finish_reason"] = "stop"

		// Try to parse response as JSON (for structured output).
		var parsed any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			output["json"] = parsed
		}
	}

	return output, nil
}

// toolCall represents an OpenAI function calling tool invocation.
type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

// toolFunction holds the function name and JSON-encoded arguments.
type toolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// chatMessage represents the assistant's response message.
type chatMessage struct {
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
