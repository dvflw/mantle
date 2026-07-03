package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
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
		if req.MaxTokens > math.MaxInt32 {
			return nil, fmt.Errorf("bedrock: max_tokens value %d exceeds maximum allowed (%d)", req.MaxTokens, math.MaxInt32)
		}
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

// classifyBedrockError maps AWS Bedrock errors into retryable vs non-retryable.
func classifyBedrockError(err error) error {
	// Log the full error server-side for debugging.
	slog.Warn("Bedrock API error", "error", err.Error())

	// Try the ErrorCode() interface first (all Bedrock API exceptions implement this).
	type errorCoder interface {
		ErrorCode() string
	}
	if ec, ok := err.(errorCoder); ok {
		switch ec.ErrorCode() {
		case "ThrottlingException", "ModelTimeoutException", "ServiceUnavailableException",
			"ModelNotReadyException", "InternalServerException":
			return &RetryableError{Err: fmt.Errorf("bedrock: service error (retryable)")}
		}
		return fmt.Errorf("bedrock: API error [%s]", ec.ErrorCode())
	}

	// Fall back to string matching for wrapped errors.
	msg := err.Error()
	for _, keyword := range []string{"ThrottlingException", "ModelTimeoutException", "ServiceUnavailableException"} {
		if strings.Contains(msg, keyword) {
			return &RetryableError{Err: fmt.Errorf("bedrock: service error (retryable)")}
		}
	}

	return fmt.Errorf("bedrock: API request failed")
}

// BedrockInvokeAPI abstracts the Bedrock InvokeModel call (used for embeddings)
// for testability.
type BedrockInvokeAPI interface {
	InvokeModel(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

// BedrockEmbeddingProvider implements EmbeddingProvider using the AWS Bedrock
// InvokeModel API. It supports two model families:
//   - Amazon Titan (amazon.titan-embed-text-v1 / -v2:0): one input per call, so
//     multi-input requests are issued sequentially and reassembled in order.
//   - Cohere Embed v3 (cohere.embed-english-v3 / cohere.embed-multilingual-v3):
//     natively batched (up to 96 texts per call) with a required input_type.
type BedrockEmbeddingProvider struct {
	Client BedrockInvokeAPI
}

// cohereEmbedMaxBatch is Cohere's per-call limit on the number of input texts.
const cohereEmbedMaxBatch = 96

// Embeddings dispatches to the right model family. The provider-agnostic
// EmbeddingRequest is mapped onto each family's InvokeModel body.
func (p *BedrockEmbeddingProvider) Embeddings(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	switch {
	case strings.HasPrefix(req.Model, "amazon.titan-embed"):
		return p.titanEmbeddings(ctx, req)
	case strings.HasPrefix(req.Model, "cohere.embed"):
		return p.cohereEmbeddings(ctx, req)
	default:
		return nil, fmt.Errorf("bedrock: unsupported embedding model %q (supported: amazon.titan-embed-text-v1, amazon.titan-embed-text-v2:0, cohere.embed-english-v3, cohere.embed-multilingual-v3)", req.Model)
	}
}

// titanEmbedRequest is the Amazon Titan text-embeddings InvokeModel body.
// Dimensions is only honoured by titan-embed-text-v2; omitempty keeps it out of
// v1 requests.
type titanEmbedRequest struct {
	InputText  string `json:"inputText"`
	Dimensions int    `json:"dimensions,omitempty"`
}

type titanEmbedResponse struct {
	Embedding           []float64 `json:"embedding"`
	InputTextTokenCount int       `json:"inputTextTokenCount"`
}

// titanEmbeddings embeds via Amazon Titan (one InvokeModel call per input).
func (p *BedrockEmbeddingProvider) titanEmbeddings(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	out := make([][]float64, len(req.Inputs))
	totalTokens := 0

	// dimensions is only accepted by Titan v2; Titan v1 (G1) rejects any field
	// other than inputText, so ignore it there rather than failing a request
	// that reuses a shared embedding config.
	supportsDimensions := strings.Contains(req.Model, "titan-embed-text-v2")
	if req.Dimensions > 0 && supportsDimensions {
		switch req.Dimensions {
		case 256, 512, 1024:
		default:
			return nil, fmt.Errorf("bedrock: %s supports dimensions 256, 512, or 1024, got %d", req.Model, req.Dimensions)
		}
	}

	for i, text := range req.Inputs {
		body := titanEmbedRequest{InputText: text}
		if req.Dimensions > 0 && supportsDimensions {
			body.Dimensions = req.Dimensions
		}
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("bedrock: marshaling embedding request: %w", err)
		}

		resp, err := p.Client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(req.Model),
			Body:        bodyJSON,
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
		})
		if err != nil {
			return nil, classifyBedrockError(err)
		}

		var er titanEmbedResponse
		if err := json.Unmarshal(resp.Body, &er); err != nil {
			return nil, fmt.Errorf("bedrock: parsing embedding response: %w", err)
		}
		if len(er.Embedding) == 0 {
			return nil, fmt.Errorf("bedrock: empty embedding returned for input %d", i)
		}
		out[i] = er.Embedding
		totalTokens += er.InputTextTokenCount
	}

	return &EmbeddingResponse{
		Embeddings: out,
		Model:      req.Model,
		Usage:      ChatUsage{PromptTokens: totalTokens, TotalTokens: totalTokens},
	}, nil
}

// cohereEmbedRequest is the Cohere Embed v3 InvokeModel body. input_type is
// required by the v3 models; truncate keeps over-length inputs from erroring.
type cohereEmbedRequest struct {
	Texts     []string `json:"texts"`
	InputType string   `json:"input_type"`
	Truncate  string   `json:"truncate,omitempty"`
}

// cohereEmbedResponse is the default ("embeddings_floats") Cohere response.
type cohereEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// cohereEmbeddings embeds via Cohere Embed v3. Cohere batches natively (up to
// cohereEmbedMaxBatch texts per call), so inputs are sent in batches and
// concatenated in order. input_type distinguishes documents from queries; it
// defaults to search_document (the ingest case). Cohere's Bedrock response
// carries no token counts, so usage is left at zero.
func (p *BedrockEmbeddingProvider) cohereEmbeddings(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	inputType := req.InputType
	switch inputType {
	case "":
		inputType = "search_document"
	case "search_document", "search_query", "classification", "clustering":
	default:
		return nil, fmt.Errorf("bedrock: cohere input_type %q is invalid (use search_document, search_query, classification, or clustering)", inputType)
	}

	out := make([][]float64, 0, len(req.Inputs))
	for start := 0; start < len(req.Inputs); start += cohereEmbedMaxBatch {
		end := start + cohereEmbedMaxBatch
		if end > len(req.Inputs) {
			end = len(req.Inputs)
		}
		batch := req.Inputs[start:end]

		bodyJSON, err := json.Marshal(cohereEmbedRequest{
			Texts:     batch,
			InputType: inputType,
			Truncate:  "END",
		})
		if err != nil {
			return nil, fmt.Errorf("bedrock: marshaling cohere embedding request: %w", err)
		}

		resp, err := p.Client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(req.Model),
			Body:        bodyJSON,
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
		})
		if err != nil {
			return nil, classifyBedrockError(err)
		}

		var er cohereEmbedResponse
		if err := json.Unmarshal(resp.Body, &er); err != nil {
			return nil, fmt.Errorf("bedrock: parsing cohere embedding response: %w", err)
		}
		if len(er.Embeddings) != len(batch) {
			return nil, fmt.Errorf("bedrock: cohere returned %d embeddings for %d inputs", len(er.Embeddings), len(batch))
		}
		out = append(out, er.Embeddings...)
	}

	return &EmbeddingResponse{
		Embeddings: out,
		Model:      req.Model,
	}, nil
}
