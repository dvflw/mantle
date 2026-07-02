package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	applyOpenAICredential(httpReq, req.Credential)

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
		slog.Warn("OpenAI API error", "status", resp.StatusCode, "body", truncate(string(body), 500))
		if resp.StatusCode == 429 {
			return nil, &RetryableError{Err: fmt.Errorf("openai: rate limited (429)")}
		}
		return nil, fmt.Errorf("openai: API returned status %d", resp.StatusCode)
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

// applyOpenAICredential sets the Authorization (and optional organization)
// headers from a resolved credential map. Shared by chat and embeddings.
func applyOpenAICredential(httpReq *http.Request, cred map[string]string) {
	if cred == nil {
		return
	}
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

// embeddingsAPIResponse is the OpenAI /embeddings response envelope.
type embeddingsAPIResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Embeddings calls the OpenAI-compatible /embeddings endpoint. Works with
// OpenAI, Azure OpenAI, and any OpenAI-compatible server via BaseURL.
func (p *OpenAIProvider) Embeddings(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	reqBody := map[string]any{
		"model": req.Model,
		"input": req.Inputs,
	}
	if req.Dimensions > 0 {
		reqBody["dimensions"] = req.Dimensions
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling embeddings request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/embeddings", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("openai: creating embeddings request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyOpenAICredential(httpReq, req.Credential)

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
		return nil, fmt.Errorf("openai: reading embeddings response: %w", err)
	}

	if resp.StatusCode != 200 {
		slog.Warn("OpenAI embeddings API error", "status", resp.StatusCode, "body", truncate(string(body), 500))
		if resp.StatusCode == 429 {
			return nil, &RetryableError{Err: fmt.Errorf("openai: rate limited (429)")}
		}
		return nil, fmt.Errorf("openai: embeddings API returned status %d", resp.StatusCode)
	}

	var apiResp embeddingsAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("openai: parsing embeddings response: %w", err)
	}
	// Reassemble in request order — the API tags each item with its index.
	// Size by the request so a short or duplicate-indexed response fails fast
	// rather than silently misaligning embeddings with their inputs.
	out := make([][]float64, len(req.Inputs))
	seen := make([]bool, len(req.Inputs))
	for _, d := range apiResp.Data {
		if d.Index < 0 || d.Index >= len(req.Inputs) {
			return nil, fmt.Errorf("openai: embedding index %d out of range for %d inputs", d.Index, len(req.Inputs))
		}
		if seen[d.Index] {
			return nil, fmt.Errorf("openai: duplicate embedding index %d", d.Index)
		}
		seen[d.Index] = true
		out[d.Index] = d.Embedding
	}
	for i, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("openai: missing embedding for input %d (got %d of %d)", i, len(apiResp.Data), len(req.Inputs))
		}
	}

	return &EmbeddingResponse{
		Embeddings: out,
		Model:      apiResp.Model,
		Usage: ChatUsage{
			PromptTokens: apiResp.Usage.PromptTokens,
			TotalTokens:  apiResp.Usage.TotalTokens,
		},
	}, nil
}
