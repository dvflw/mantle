package connector

import "context"

// LLMProvider implements a specific AI provider's chat completion API.
type LLMProvider interface {
	ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

// ChatRequest is the provider-agnostic request format.
type ChatRequest struct {
	Model        string
	Messages     []ChatMessage
	Tools        []ChatTool
	OutputSchema map[string]any
	MaxTokens    int
	Credential   map[string]string
}

// ChatMessage represents a single message in the conversation.
// Valid Role values: "system", "user", "assistant", "tool".
// For "tool" messages (tool results), set ToolCallID to match the original call.
// Note: "tool" role maps to ConversationRoleUser in Bedrock (tool results are user messages).
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall // reuses existing ToolCall type from ai.go
	ToolCallID string
}

// ChatTool defines a tool the model can invoke.
type ChatTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// ChatResponse is the provider-agnostic response format.
type ChatResponse struct {
	Text         string
	ToolCalls    []ToolCall
	FinishReason string // "stop" or "tool_calls"
	Usage        ChatUsage
	Model        string
}

// ChatUsage tracks token consumption.
type ChatUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
