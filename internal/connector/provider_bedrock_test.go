package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	brdocument "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
)

// mockBedrockClient implements BedrockConverseAPI for testing.
type mockBedrockClient struct {
	// ConverseFunc is called when Converse is invoked.
	ConverseFunc func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

func (m *mockBedrockClient) Converse(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	return m.ConverseFunc(ctx, input, opts...)
}

func TestBedrockProvider_BasicCompletion(t *testing.T) {
	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			// Verify model ID.
			if aws.ToString(input.ModelId) != "anthropic.claude-3-sonnet-20240229-v1:0" {
				t.Errorf("ModelId = %q, want anthropic.claude-3-sonnet-20240229-v1:0", aws.ToString(input.ModelId))
			}
			// Verify messages.
			if len(input.Messages) != 1 {
				t.Fatalf("len(Messages) = %d, want 1", len(input.Messages))
			}
			if input.Messages[0].Role != brtypes.ConversationRoleUser {
				t.Errorf("Messages[0].Role = %q, want user", input.Messages[0].Role)
			}
			// Verify system prompt.
			if len(input.System) != 1 {
				t.Fatalf("len(System) = %d, want 1", len(input.System))
			}

			return &bedrockruntime.ConverseOutput{
				Output: &brtypes.ConverseOutputMemberMessage{
					Value: brtypes.Message{
						Role: brtypes.ConversationRoleAssistant,
						Content: []brtypes.ContentBlock{
							&brtypes.ContentBlockMemberText{Value: "Hello from Bedrock!"},
						},
					},
				},
				StopReason: brtypes.StopReasonEndTurn,
				Usage: &brtypes.TokenUsage{
					InputTokens:  aws.Int32(10),
					OutputTokens: aws.Int32(5),
					TotalTokens:  aws.Int32(15),
				},
			}, nil
		},
	}

	p := &BedrockProvider{Client: mock}
	resp, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model: "anthropic.claude-3-sonnet-20240229-v1:0",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.Text != "Hello from Bedrock!" {
		t.Errorf("Text = %q, want %q", resp.Text, "Hello from Bedrock!")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("CompletionTokens = %d, want 5", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
	if resp.Model != "anthropic.claude-3-sonnet-20240229-v1:0" {
		t.Errorf("Model = %q, want %q", resp.Model, "anthropic.claude-3-sonnet-20240229-v1:0")
	}
}

func TestBedrockProvider_ToolCalls(t *testing.T) {
	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			// Verify tools were passed.
			if input.ToolConfig == nil || len(input.ToolConfig.Tools) != 1 {
				t.Fatalf("expected 1 tool, got %v", input.ToolConfig)
			}

			return &bedrockruntime.ConverseOutput{
				Output: &brtypes.ConverseOutputMemberMessage{
					Value: brtypes.Message{
						Role: brtypes.ConversationRoleAssistant,
						Content: []brtypes.ContentBlock{
							&brtypes.ContentBlockMemberToolUse{
								Value: brtypes.ToolUseBlock{
									ToolUseId: aws.String("tooluse_abc123"),
									Name:      aws.String("get_weather"),
									Input:     brdocument.NewLazyDocument(map[string]any{"city": "NYC"}),
								},
							},
						},
					},
				},
				StopReason: brtypes.StopReasonToolUse,
				Usage: &brtypes.TokenUsage{
					InputTokens:  aws.Int32(20),
					OutputTokens: aws.Int32(15),
					TotalTokens:  aws.Int32(35),
				},
			}, nil
		},
	}

	p := &BedrockProvider{Client: mock}
	resp, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model: "anthropic.claude-3-sonnet-20240229-v1:0",
		Messages: []ChatMessage{
			{Role: "user", Content: "What is the weather in NYC?"},
		},
		Tools: []ChatTool{
			{
				Name:        "get_weather",
				Description: "Get current weather",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_calls")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "tooluse_abc123" {
		t.Errorf("ToolCalls[0].ID = %q, want %q", tc.ID, "tooluse_abc123")
	}
	if tc.Type != "function" {
		t.Errorf("ToolCalls[0].Type = %q, want %q", tc.Type, "function")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("ToolCalls[0].Function.Name = %q, want %q", tc.Function.Name, "get_weather")
	}
	// Arguments should be JSON with city.
	if !strings.Contains(tc.Function.Arguments, "NYC") {
		t.Errorf("ToolCalls[0].Function.Arguments = %q, want to contain NYC", tc.Function.Arguments)
	}
}

func TestBedrockProvider_StructuredOutput(t *testing.T) {
	var capturedInput *bedrockruntime.ConverseInput
	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedInput = input
			return &bedrockruntime.ConverseOutput{
				Output: &brtypes.ConverseOutputMemberMessage{
					Value: brtypes.Message{
						Role: brtypes.ConversationRoleAssistant,
						Content: []brtypes.ContentBlock{
							&brtypes.ContentBlockMemberText{Value: `{"result":"ok"}`},
						},
					},
				},
				StopReason: brtypes.StopReasonEndTurn,
				Usage: &brtypes.TokenUsage{
					InputTokens:  aws.Int32(0),
					OutputTokens: aws.Int32(0),
					TotalTokens:  aws.Int32(0),
				},
			}, nil
		},
	}

	p := &BedrockProvider{Client: mock}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "anthropic.claude-3-sonnet-20240229-v1:0",
		Messages: []ChatMessage{{Role: "user", Content: "Test"}},
		OutputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"result": map[string]any{"type": "string"}},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	// Verify system prompt includes schema instruction.
	if capturedInput == nil {
		t.Fatal("input was not captured")
	}
	if len(capturedInput.System) == 0 {
		t.Fatal("expected system blocks for structured output, got none")
	}

	// Find the schema instruction in system blocks.
	found := false
	for _, block := range capturedInput.System {
		if tb, ok := block.(*brtypes.SystemContentBlockMemberText); ok {
			if strings.Contains(tb.Value, "JSON") && strings.Contains(tb.Value, "result") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected system block with JSON schema instruction")
	}
}

func TestBedrockProvider_ThrottlingError(t *testing.T) {
	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, &brtypes.ThrottlingException{
				Message: aws.String("Rate exceeded"),
			}
		},
	}

	p := &BedrockProvider{Client: mock}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "anthropic.claude-3-sonnet-20240229-v1:0",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var retryErr *RetryableError
	if !errors.As(err, &retryErr) {
		t.Errorf("expected RetryableError, got %T: %v", err, err)
	}
}

func TestBedrockProvider_AccessDenied(t *testing.T) {
	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, &brtypes.AccessDeniedException{
				Message: aws.String("Not authorized"),
			}
		},
	}

	p := &BedrockProvider{Client: mock}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "anthropic.claude-3-sonnet-20240229-v1:0",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should NOT be retryable.
	var retryErr *RetryableError
	if errors.As(err, &retryErr) {
		t.Errorf("AccessDeniedException should not be retryable, got RetryableError")
	}

	// Should still contain the original error.
	if !strings.Contains(err.Error(), "AccessDeniedException") {
		t.Errorf("error should mention AccessDeniedException: %v", err)
	}
}

func TestBedrockProvider_InferenceProfileARN(t *testing.T) {
	var capturedModelId string
	arn := "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-sonnet-20240229-v1:0"

	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			capturedModelId = aws.ToString(input.ModelId)
			return &bedrockruntime.ConverseOutput{
				Output: &brtypes.ConverseOutputMemberMessage{
					Value: brtypes.Message{
						Role: brtypes.ConversationRoleAssistant,
						Content: []brtypes.ContentBlock{
							&brtypes.ContentBlockMemberText{Value: "Hello"},
						},
					},
				},
				StopReason: brtypes.StopReasonEndTurn,
				Usage: &brtypes.TokenUsage{
					InputTokens:  aws.Int32(0),
					OutputTokens: aws.Int32(0),
					TotalTokens:  aws.Int32(0),
				},
			}, nil
		},
	}

	p := &BedrockProvider{Client: mock}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    arn,
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if capturedModelId != arn {
		t.Errorf("ModelId = %q, want %q", capturedModelId, arn)
	}
}

func TestBedrockProvider_ToolResultMessage(t *testing.T) {
	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			// Verify tool result is sent as user message with ToolResultBlock.
			if len(input.Messages) != 3 {
				t.Fatalf("len(Messages) = %d, want 3", len(input.Messages))
			}
			// Third message should be a user message (tool result).
			toolResultMsg := input.Messages[2]
			if toolResultMsg.Role != brtypes.ConversationRoleUser {
				t.Errorf("tool result message Role = %q, want user", toolResultMsg.Role)
			}
			if len(toolResultMsg.Content) != 1 {
				t.Fatalf("tool result content blocks = %d, want 1", len(toolResultMsg.Content))
			}
			trBlock, ok := toolResultMsg.Content[0].(*brtypes.ContentBlockMemberToolResult)
			if !ok {
				t.Fatalf("expected ContentBlockMemberToolResult, got %T", toolResultMsg.Content[0])
			}
			if aws.ToString(trBlock.Value.ToolUseId) != "call_123" {
				t.Errorf("ToolUseId = %q, want %q", aws.ToString(trBlock.Value.ToolUseId), "call_123")
			}

			return &bedrockruntime.ConverseOutput{
				Output: &brtypes.ConverseOutputMemberMessage{
					Value: brtypes.Message{
						Role: brtypes.ConversationRoleAssistant,
						Content: []brtypes.ContentBlock{
							&brtypes.ContentBlockMemberText{Value: "The weather is sunny."},
						},
					},
				},
				StopReason: brtypes.StopReasonEndTurn,
				Usage: &brtypes.TokenUsage{
					InputTokens:  aws.Int32(30),
					OutputTokens: aws.Int32(10),
					TotalTokens:  aws.Int32(40),
				},
			}, nil
		},
	}

	p := &BedrockProvider{Client: mock}
	resp, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model: "anthropic.claude-3-sonnet-20240229-v1:0",
		Messages: []ChatMessage{
			{Role: "user", Content: "What is the weather?"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{
				{ID: "call_123", Type: "function", Function: ToolFunction{Name: "get_weather", Arguments: `{"city":"NYC"}`}},
			}},
			{Role: "tool", Content: `{"temp": 72}`, ToolCallID: "call_123"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.Text != "The weather is sunny." {
		t.Errorf("Text = %q, want %q", resp.Text, "The weather is sunny.")
	}
}

func TestBedrockProvider_StringFallbackError(t *testing.T) {
	// Test error classification via string matching (for wrapped errors).
	mock := &mockBedrockClient{
		ConverseFunc: func(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
			return nil, fmt.Errorf("operation failed: ThrottlingException: rate exceeded")
		},
	}

	p := &BedrockProvider{Client: mock}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "anthropic.claude-3-sonnet-20240229-v1:0",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var retryErr *RetryableError
	if !errors.As(err, &retryErr) {
		t.Errorf("expected RetryableError from string fallback, got %T: %v", err, err)
	}
}
