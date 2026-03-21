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

// OpenAIProvider implements LLMProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	Client  *http.Client
	BaseURL string
}

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Build messages in OpenAI wire format.
	var messages []any
	for _, m := range req.Messages {
		msg := map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = SerializeToolCalls(m.ToolCalls)
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		messages = append(messages, msg)
	}

	reqBody := map[string]any{
		"model":    req.Model,
		"messages": messages,
	}

	// Include tools.
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			})
		}
		reqBody["tools"] = tools
	}

	// Structured output via response_format.
	if req.OutputSchema != nil {
		reqBody["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "response",
				"strict": true,
				"schema": req.OutputSchema,
			},
		}
	}

	if req.MaxTokens > 0 {
		reqBody["max_tokens"] = req.MaxTokens
	}

	endpoint := p.BaseURL + "/chat/completions"

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("openai: creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Apply credential-based auth.
	if cred := req.Credential; cred != nil {
		switch {
		case cred["api_key"] != "":
			httpReq.Header.Set("Authorization", "Bearer "+cred["api_key"])
		case cred["token"] != "":
			httpReq.Header.Set("Authorization", "Bearer "+cred["token"])
		case cred["key"] != "":
			httpReq.Header.Set("Authorization", "Bearer "+cred["key"])
		}
		if orgID := cred["org_id"]; orgID != "" {
			httpReq.Header.Set("OpenAI-Organization", orgID)
		}
	}

	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var apiResp chatCompletionResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("openai: parsing response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices returned")
	}

	choice := apiResp.Choices[0]

	chatResp := &ChatResponse{
		Model: apiResp.Model,
		Usage: ChatUsage{
			PromptTokens:     apiResp.Usage.PromptTokens,
			CompletionTokens: apiResp.Usage.CompletionTokens,
			TotalTokens:      apiResp.Usage.TotalTokens,
		},
	}

	if len(choice.Message.ToolCalls) > 0 {
		chatResp.ToolCalls = choice.Message.ToolCalls
		chatResp.FinishReason = "tool_calls"
	} else {
		chatResp.Text = choice.Message.Content
		chatResp.FinishReason = "stop"
	}

	return chatResp, nil
}
