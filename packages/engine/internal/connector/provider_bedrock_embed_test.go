package connector

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// mockBedrockInvokeClient implements BedrockInvokeAPI for testing.
type mockBedrockInvokeClient struct {
	InvokeFunc func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

func (m *mockBedrockInvokeClient) InvokeModel(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	return m.InvokeFunc(ctx, input, opts...)
}

func TestBedrockEmbeddingProvider_Titan(t *testing.T) {
	var calls int
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			calls++
			if aws.ToString(input.ModelId) != "amazon.titan-embed-text-v2:0" {
				t.Errorf("ModelId = %q", aws.ToString(input.ModelId))
			}
			// Verify the request body carries inputText and the requested dimensions.
			var body titanEmbedRequest
			if err := json.Unmarshal(input.Body, &body); err != nil {
				t.Fatalf("unmarshaling request body: %v", err)
			}
			if body.InputText == "" {
				t.Error("inputText is empty")
			}
			if body.Dimensions != 256 {
				t.Errorf("dimensions = %d, want 256", body.Dimensions)
			}
			resp, _ := json.Marshal(titanEmbedResponse{
				Embedding:           []float64{0.5, -0.25},
				InputTextTokenCount: 4,
			})
			return &bedrockruntime.InvokeModelOutput{Body: resp}, nil
		},
	}

	p := &BedrockEmbeddingProvider{Client: mock}
	resp, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:      "amazon.titan-embed-text-v2:0",
		Inputs:     []string{"one", "two", "three"},
		Dimensions: 256,
	})
	if err != nil {
		t.Fatalf("Embeddings() error: %v", err)
	}
	// Titan embeds one input per call.
	if calls != 3 {
		t.Errorf("InvokeModel calls = %d, want 3", calls)
	}
	if len(resp.Embeddings) != 3 {
		t.Fatalf("embeddings = %d, want 3", len(resp.Embeddings))
	}
	if resp.Embeddings[0][0] != 0.5 || resp.Embeddings[0][1] != -0.25 {
		t.Errorf("embedding[0] = %v", resp.Embeddings[0])
	}
	if resp.Usage.TotalTokens != 12 { // 4 per input * 3
		t.Errorf("TotalTokens = %d, want 12", resp.Usage.TotalTokens)
	}
}

func TestBedrockEmbeddingProvider_V1IgnoresDimensions(t *testing.T) {
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			var body titanEmbedRequest
			if err := json.Unmarshal(input.Body, &body); err != nil {
				t.Fatalf("unmarshaling request body: %v", err)
			}
			// Titan v1 must not receive dimensions even when requested.
			if body.Dimensions != 0 {
				t.Errorf("dimensions = %d, want 0 (omitted) for titan v1", body.Dimensions)
			}
			resp, _ := json.Marshal(titanEmbedResponse{Embedding: []float64{1}, InputTextTokenCount: 1})
			return &bedrockruntime.InvokeModelOutput{Body: resp}, nil
		},
	}
	p := &BedrockEmbeddingProvider{Client: mock}
	if _, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:      "amazon.titan-embed-text-v1",
		Inputs:     []string{"x"},
		Dimensions: 512,
	}); err != nil {
		t.Fatalf("Embeddings() error: %v", err)
	}
}

func TestBedrockEmbeddingProvider_V2InvalidDimensions(t *testing.T) {
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			t.Fatal("InvokeModel should not be called for invalid dimensions")
			return nil, nil
		},
	}
	p := &BedrockEmbeddingProvider{Client: mock}
	_, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:      "amazon.titan-embed-text-v2:0",
		Inputs:     []string{"x"},
		Dimensions: 999, // only 256/512/1024 are valid
	})
	if err == nil {
		t.Error("expected error for unsupported Titan v2 dimensions")
	}
}

func TestBedrockEmbeddingProvider_UnsupportedModel(t *testing.T) {
	p := &BedrockEmbeddingProvider{Client: &mockBedrockInvokeClient{}}
	_, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:  "cohere.embed-english-v3",
		Inputs: []string{"x"},
	})
	if err == nil {
		t.Error("expected error for unsupported (non-Titan) Bedrock embedding model")
	}
}
