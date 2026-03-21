package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	brdocument "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
)

// BedrockConverseAPI abstracts the Bedrock Converse call for testability.
type BedrockConverseAPI interface {
	Converse(ctx context.Context, input *bedrockruntime.ConverseInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

// BedrockProvider implements LLMProvider using the AWS Bedrock Converse API.
type BedrockProvider struct {
	Client BedrockConverseAPI
}

func (p *BedrockProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	input := &bedrockruntime.ConverseInput{
		ModelId: aws.String(req.Model),
	}

	// Separate system messages from conversation messages.
	var systemBlocks []brtypes.SystemContentBlock
	var messages []brtypes.Message

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			systemBlocks = append(systemBlocks, &brtypes.SystemContentBlockMemberText{
				Value: m.Content,
			})

		case "user":
			messages = append(messages, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: m.Content}},
			})

		case "assistant":
			var content []brtypes.ContentBlock
			if m.Content != "" {
				content = append(content, &brtypes.ContentBlockMemberText{Value: m.Content})
			}
			for _, tc := range m.ToolCalls {
				var toolInput any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &toolInput); err != nil {
					toolInput = map[string]any{}
				}
				content = append(content, &brtypes.ContentBlockMemberToolUse{
					Value: brtypes.ToolUseBlock{
						ToolUseId: aws.String(tc.ID),
						Name:      aws.String(tc.Function.Name),
						Input:     brdocument.NewLazyDocument(toolInput),
					},
				})
			}
			messages = append(messages, brtypes.Message{
				Role:    brtypes.ConversationRoleAssistant,
				Content: content,
			})

		case "tool":
			// Tool results are sent as user messages in Bedrock.
			messages = append(messages, brtypes.Message{
				Role: brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{
					&brtypes.ContentBlockMemberToolResult{
						Value: brtypes.ToolResultBlock{
							ToolUseId: aws.String(m.ToolCallID),
							Content: []brtypes.ToolResultContentBlock{
								&brtypes.ToolResultContentBlockMemberText{Value: m.Content},
							},
						},
					},
				},
			})
		}
	}

	// If output_schema is set, inject a JSON schema instruction into system prompt.
	// Bedrock does not have a native json_schema response_format like OpenAI.
	if req.OutputSchema != nil {
		schemaJSON, err := json.Marshal(req.OutputSchema)
		if err == nil {
			instruction := fmt.Sprintf(
				"You MUST respond with valid JSON matching this schema:\n%s\nDo not include any text outside the JSON object.",
				string(schemaJSON),
			)
			systemBlocks = append(systemBlocks, &brtypes.SystemContentBlockMemberText{
				Value: instruction,
			})
		}
	}

	if len(systemBlocks) > 0 {
		input.System = systemBlocks
	}
	input.Messages = messages

	// Convert tools to Bedrock ToolConfiguration.
	if len(req.Tools) > 0 {
		var tools []brtypes.Tool
		for _, t := range req.Tools {
			tools = append(tools, &brtypes.ToolMemberToolSpec{
				Value: brtypes.ToolSpecification{
					Name:        aws.String(t.Name),
					Description: aws.String(t.Description),
					InputSchema: &brtypes.ToolInputSchemaMemberJson{
						Value: brdocument.NewLazyDocument(t.InputSchema),
					},
				},
			})
		}
		input.ToolConfig = &brtypes.ToolConfiguration{
			Tools: tools,
		}
	}

	// Set max tokens if specified.
	if req.MaxTokens > 0 {
		input.InferenceConfig = &brtypes.InferenceConfiguration{
			MaxTokens: aws.Int32(int32(req.MaxTokens)),
		}
	}

	output, err := p.Client.Converse(ctx, input)
	if err != nil {
		return nil, classifyBedrockError(err)
	}

	resp := &ChatResponse{
		Model: req.Model,
	}

	// Extract usage.
	if output.Usage != nil {
		resp.Usage = ChatUsage{
			PromptTokens:     int(aws.ToInt32(output.Usage.InputTokens)),
			CompletionTokens: int(aws.ToInt32(output.Usage.OutputTokens)),
			TotalTokens:      int(aws.ToInt32(output.Usage.TotalTokens)),
		}
	}

	// Extract output message.
	msgOutput, ok := output.Output.(*brtypes.ConverseOutputMemberMessage)
	if !ok {
		return nil, fmt.Errorf("bedrock: unexpected output type %T", output.Output)
	}

	if output.StopReason == brtypes.StopReasonToolUse {
		resp.FinishReason = "tool_calls"
		for _, block := range msgOutput.Value.Content {
			if toolUse, ok := block.(*brtypes.ContentBlockMemberToolUse); ok {
				argsBytes := []byte("{}")
				if toolUse.Value.Input != nil {
					if b, err := toolUse.Value.Input.MarshalSmithyDocument(); err == nil && len(b) > 0 {
						argsBytes = b
					}
				}
				resp.ToolCalls = append(resp.ToolCalls, ToolCall{
					ID:   aws.ToString(toolUse.Value.ToolUseId),
					Type: "function",
					Function: ToolFunction{
						Name:      aws.ToString(toolUse.Value.Name),
						Arguments: string(argsBytes),
					},
				})
			}
		}
	} else {
		resp.FinishReason = "stop"
		var textParts []string
		for _, block := range msgOutput.Value.Content {
			if textBlock, ok := block.(*brtypes.ContentBlockMemberText); ok {
				textParts = append(textParts, textBlock.Value)
			}
		}
		resp.Text = strings.Join(textParts, "")
	}

	return resp, nil
}

// RetryableError wraps an error to indicate the caller should retry.
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable: %v", e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// classifyBedrockError maps AWS Bedrock errors into retryable vs non-retryable.
func classifyBedrockError(err error) error {
	// Try the ErrorCode() interface first (all Bedrock API exceptions implement this).
	type errorCoder interface {
		ErrorCode() string
	}
	if ec, ok := err.(errorCoder); ok {
		switch ec.ErrorCode() {
		case "ThrottlingException", "ModelTimeoutException", "ServiceUnavailableException",
			"ModelNotReadyException", "InternalServerException":
			return &RetryableError{Err: err}
		}
		return fmt.Errorf("bedrock: %w", err)
	}

	// Fall back to string matching for wrapped errors.
	msg := err.Error()
	for _, keyword := range []string{"ThrottlingException", "ModelTimeoutException", "ServiceUnavailableException"} {
		if strings.Contains(msg, keyword) {
			return &RetryableError{Err: err}
		}
	}

	return fmt.Errorf("bedrock: %w", err)
}
