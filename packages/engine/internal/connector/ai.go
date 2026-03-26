package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/dvflw/mantle/internal/metrics"
)

// AIConnector dispatches chat completion requests to the appropriate LLMProvider.
type AIConnector struct {
	Client          *http.Client
	AWSConfigFunc   func(ctx context.Context, cred map[string]string, defaultRegion string) (aws.Config, error)
	DefaultRegion   string
	AllowedBaseURLs []string
	AllowedModels   []string // empty = all models allowed
}

func (c *AIConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	providerName, _ := params["provider"].(string)
	if providerName == "" {
		providerName = "openai"
	}

	provider, err := c.getProvider(providerName, params)
	if err != nil {
		return nil, fmt.Errorf("ai/completion: %w", err)
	}

	req, err := buildChatRequest(params)
	if err != nil {
		return nil, fmt.Errorf("ai/completion: %w", err)
	}

	model := req.Model

	// Enforce model allowlist if configured.
	if len(c.AllowedModels) > 0 {
		allowed := false
		for _, m := range c.AllowedModels {
			if m == model {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("ai/completion: model %q not in allowed list", model)
		}
	}
	workflow, _ := params["_workflow"].(string)
	step, _ := params["_step"].(string)

	start := time.Now()
	resp, err := provider.ChatCompletion(ctx, req)
	duration := time.Since(start).Seconds()

	// Record metrics.
	metrics.AIRequestDuration.WithLabelValues(workflow, step, model, providerName).Observe(duration)
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues(workflow, step, model, providerName, "error").Inc()
		return nil, fmt.Errorf("ai/completion: %w", err)
	}
	if resp != nil {
		metrics.AITokensTotal.WithLabelValues(workflow, step, model, providerName, "prompt").Add(float64(resp.Usage.PromptTokens))
		metrics.AITokensTotal.WithLabelValues(workflow, step, model, providerName, "completion").Add(float64(resp.Usage.CompletionTokens))
		metrics.AIRequestsTotal.WithLabelValues(workflow, step, model, providerName, "success").Inc()
	}

	return chatResponseToOutput(resp), nil
}

// getProvider returns the LLMProvider for the given provider name.
func (c *AIConnector) getProvider(name string, params map[string]any) (LLMProvider, error) {
	switch name {
	case "openai":
		baseURL := "https://api.openai.com/v1"
		if u, ok := params["base_url"].(string); ok && u != "" {
			baseURL = u
		}
		if len(c.AllowedBaseURLs) > 0 && baseURL != "" {
			allowed := false
			for _, u := range c.AllowedBaseURLs {
				if u == baseURL {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, fmt.Errorf("ai/completion: base_url %q not in allowed list", baseURL)
			}
		}
		return &OpenAIProvider{
			Client:  c.Client,
			BaseURL: baseURL,
		}, nil
	case "bedrock":
		cred, _ := params["_credential"].(map[string]string)
		region, _ := params["region"].(string)
		defaultRegion := c.DefaultRegion
		if region != "" {
			defaultRegion = region
		}
		configFunc := c.AWSConfigFunc
		if configFunc == nil {
			configFunc = NewAWSConfig
		}
		awsCfg, err := configFunc(context.Background(), cred, defaultRegion)
		if err != nil {
			return nil, fmt.Errorf("ai/completion [bedrock]: %w", err)
		}
		client := bedrockruntime.NewFromConfig(awsCfg)
		return &BedrockProvider{Client: client}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (available: openai, bedrock)", name)
	}
}

// buildChatRequest converts raw params into a provider-agnostic ChatRequest.
func buildChatRequest(params map[string]any) (*ChatRequest, error) {
	model, _ := params["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	req := &ChatRequest{
		Model: model,
	}

	// Build messages from _messages passthrough or prompt/system_prompt.
	if rawMessages, ok := params["_messages"]; ok {
		req.Messages = convertRawMessages(rawMessages)
	} else {
		prompt, _ := params["prompt"].(string)
		if prompt == "" {
			return nil, fmt.Errorf("prompt is required")
		}
		if systemPrompt, _ := params["system_prompt"].(string); systemPrompt != "" {
			req.Messages = append(req.Messages, ChatMessage{Role: "system", Content: systemPrompt})
		}
		req.Messages = append(req.Messages, ChatMessage{Role: "user", Content: prompt})
	}

	// Tools from _tools passthrough.
	if rawTools, ok := params["_tools"]; ok {
		req.Tools = convertRawTools(rawTools)
	}

	// Output schema.
	if schema, ok := params["output_schema"].(map[string]any); ok {
		req.OutputSchema = schema
	}

	// Credential.
	if cred, ok := params["_credential"].(map[string]string); ok {
		req.Credential = cred
		delete(params, "_credential")
	}

	// Max tokens.
	if mt, ok := params["max_tokens"]; ok {
		if v, ok2 := extractInt(mt); ok2 {
			req.MaxTokens = v
		}
	}

	return req, nil
}

// convertRawMessages converts the raw _messages param (various formats) into []ChatMessage.
// It handles []map[string]any (from ToolLoop) and preserves all fields.
func convertRawMessages(raw any) []ChatMessage {
	switch msgs := raw.(type) {
	case []map[string]any:
		return convertMapMessages(msgs)
	case []any:
		var out []ChatMessage
		for _, m := range msgs {
			if mm, ok := m.(map[string]any); ok {
				out = append(out, mapToMessage(mm))
			}
		}
		return out
	default:
		return nil
	}
}

func convertMapMessages(msgs []map[string]any) []ChatMessage {
	out := make([]ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, mapToMessage(m))
	}
	return out
}

func mapToMessage(m map[string]any) ChatMessage {
	msg := ChatMessage{
		Role:    stringVal(m, "role"),
		Content: stringVal(m, "content"),
	}
	if tcID, ok := m["tool_call_id"].(string); ok {
		msg.ToolCallID = tcID
	}
	if rawTC, ok := m["tool_calls"]; ok {
		msg.ToolCalls = convertRawToolCalls(rawTC)
	}
	return msg
}

func stringVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// convertRawToolCalls converts tool_calls from various wire formats into []ToolCall.
func convertRawToolCalls(raw any) []ToolCall {
	switch tcs := raw.(type) {
	case []ToolCall:
		return tcs
	case []map[string]any:
		out := make([]ToolCall, 0, len(tcs))
		for _, tc := range tcs {
			call := ToolCall{
				ID:   stringVal(tc, "id"),
				Type: stringVal(tc, "type"),
			}
			if fn, ok := tc["function"].(map[string]any); ok {
				call.Function.Name = stringVal(fn, "name")
				call.Function.Arguments = stringVal(fn, "arguments")
			}
			out = append(out, call)
		}
		return out
	case []any:
		out := make([]ToolCall, 0, len(tcs))
		for _, item := range tcs {
			if tc, ok := item.(map[string]any); ok {
				call := ToolCall{
					ID:   stringVal(tc, "id"),
					Type: stringVal(tc, "type"),
				}
				if fn, ok := tc["function"].(map[string]any); ok {
					call.Function.Name = stringVal(fn, "name")
					call.Function.Arguments = stringVal(fn, "arguments")
				}
				out = append(out, call)
			}
		}
		return out
	default:
		return nil
	}
}

// convertRawTools converts the raw _tools param into []ChatTool.
func convertRawTools(raw any) []ChatTool {
	switch tools := raw.(type) {
	case []map[string]any:
		return mapSliceToTools(tools)
	case []any:
		var maps []map[string]any
		for _, t := range tools {
			if m, ok := t.(map[string]any); ok {
				maps = append(maps, m)
			}
		}
		return mapSliceToTools(maps)
	default:
		return nil
	}
}

func mapSliceToTools(tools []map[string]any) []ChatTool {
	var out []ChatTool
	for _, t := range tools {
		// OpenAI format: {type: "function", function: {name, description, parameters}}
		if fn, ok := t["function"].(map[string]any); ok {
			tool := ChatTool{
				Name:        stringVal(fn, "name"),
				Description: stringVal(fn, "description"),
			}
			if schema, ok := fn["parameters"].(map[string]any); ok {
				tool.InputSchema = schema
			}
			out = append(out, tool)
		}
	}
	return out
}

// chatResponseToOutput converts a ChatResponse back to the map[string]any output format.
func chatResponseToOutput(resp *ChatResponse) map[string]any {
	output := map[string]any{
		"model": resp.Model,
		"usage": map[string]any{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		},
	}

	if len(resp.ToolCalls) > 0 {
		output["tool_calls"] = resp.ToolCalls
		output["finish_reason"] = "tool_calls"
	} else {
		output["text"] = resp.Text
		output["finish_reason"] = "stop"

		// Try to parse response as JSON (for structured output).
		var parsed any
		if json.Unmarshal([]byte(resp.Text), &parsed) == nil {
			output["json"] = parsed
		}
	}

	return output
}

// ToolCall represents an OpenAI function calling tool invocation.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction holds the function name and JSON-encoded arguments.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// chatMessage represents the assistant's response message.
type chatMessage struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
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
