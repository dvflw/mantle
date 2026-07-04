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
		Model:  "meta.llama-embed-v1",
		Inputs: []string{"x"},
	})
	if err == nil {
		t.Error("expected error for unsupported Bedrock embedding model family")
	}
}

func TestBedrockEmbeddingProvider_UnsupportedCohereModel(t *testing.T) {
	// A valid-but-unsupported Cohere ID (e.g. v4) must not be routed into the v3
	// request path; it should return the unsupported-model error instead.
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			t.Fatal("InvokeModel should not be called for an unsupported Cohere model")
			return nil, nil
		},
	}
	p := &BedrockEmbeddingProvider{Client: mock}
	if _, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:  "cohere.embed-v4:0",
		Inputs: []string{"x"},
	}); err == nil {
		t.Error("expected error for unsupported Cohere embedding model (v4)")
	}
}

func TestBedrockEmbeddingProvider_Cohere(t *testing.T) {
	var calls int
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			calls++
			if aws.ToString(input.ModelId) != "cohere.embed-english-v3" {
				t.Errorf("ModelId = %q", aws.ToString(input.ModelId))
			}
			var body cohereEmbedRequest
			if err := json.Unmarshal(input.Body, &body); err != nil {
				t.Fatalf("unmarshaling request body: %v", err)
			}
			// Cohere batches: all inputs arrive in one call with the input_type.
			if len(body.Texts) != 3 {
				t.Errorf("texts = %d, want 3 (single batched call)", len(body.Texts))
			}
			if body.InputType != "search_query" {
				t.Errorf("input_type = %q, want search_query", body.InputType)
			}
			embs := make([][]float64, len(body.Texts))
			for i := range embs {
				embs[i] = []float64{float64(i), 0.5}
			}
			resp, _ := json.Marshal(cohereEmbedResponse{Embeddings: embs})
			return &bedrockruntime.InvokeModelOutput{Body: resp}, nil
		},
	}

	p := &BedrockEmbeddingProvider{Client: mock}
	resp, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:     "cohere.embed-english-v3",
		Inputs:    []string{"one", "two", "three"},
		InputType: "search_query",
	})
	if err != nil {
		t.Fatalf("Embeddings() error: %v", err)
	}
	if calls != 1 {
		t.Errorf("InvokeModel calls = %d, want 1 (Cohere batches)", calls)
	}
	if len(resp.Embeddings) != 3 {
		t.Fatalf("embeddings = %d, want 3", len(resp.Embeddings))
	}
	if resp.Embeddings[2][0] != 2 {
		t.Errorf("embedding[2] = %v, want order preserved", resp.Embeddings[2])
	}
}

func TestBedrockEmbeddingProvider_CohereDefaultInputType(t *testing.T) {
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			var body cohereEmbedRequest
			if err := json.Unmarshal(input.Body, &body); err != nil {
				t.Fatalf("unmarshaling request body: %v", err)
			}
			// Absent input_type defaults to search_document (the ingest case).
			if body.InputType != "search_document" {
				t.Errorf("input_type = %q, want default search_document", body.InputType)
			}
			resp, _ := json.Marshal(cohereEmbedResponse{Embeddings: [][]float64{{1}}})
			return &bedrockruntime.InvokeModelOutput{Body: resp}, nil
		},
	}
	p := &BedrockEmbeddingProvider{Client: mock}
	if _, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:  "cohere.embed-multilingual-v3",
		Inputs: []string{"x"},
	}); err != nil {
		t.Fatalf("Embeddings() error: %v", err)
	}
}

func TestBedrockEmbeddingProvider_CohereInvalidInputType(t *testing.T) {
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			t.Fatal("InvokeModel should not be called for an invalid input_type")
			return nil, nil
		},
	}
	p := &BedrockEmbeddingProvider{Client: mock}
	_, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:     "cohere.embed-english-v3",
		Inputs:    []string{"x"},
		InputType: "bogus",
	})
	if err == nil {
		t.Error("expected error for invalid cohere input_type")
	}
}

func TestBedrockEmbeddingProvider_CohereBatches(t *testing.T) {
	// More than one batch worth of inputs must be split across calls and
	// reassembled in order.
	var calls int
	mock := &mockBedrockInvokeClient{
		InvokeFunc: func(ctx context.Context, input *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
			calls++
			var body cohereEmbedRequest
			if err := json.Unmarshal(input.Body, &body); err != nil {
				t.Fatalf("unmarshaling request body: %v", err)
			}
			if len(body.Texts) > cohereEmbedMaxBatch {
				t.Errorf("batch of %d exceeds max %d", len(body.Texts), cohereEmbedMaxBatch)
			}
			embs := make([][]float64, len(body.Texts))
			for i := range embs {
				embs[i] = []float64{1}
			}
			resp, _ := json.Marshal(cohereEmbedResponse{Embeddings: embs})
			return &bedrockruntime.InvokeModelOutput{Body: resp}, nil
		},
	}
	inputs := make([]string, cohereEmbedMaxBatch+5)
	for i := range inputs {
		inputs[i] = "t"
	}
	p := &BedrockEmbeddingProvider{Client: mock}
	resp, err := p.Embeddings(context.Background(), &EmbeddingRequest{
		Model:  "cohere.embed-english-v3",
		Inputs: inputs,
	})
	if err != nil {
		t.Fatalf("Embeddings() error: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (%d + 5 split into two batches)", calls, cohereEmbedMaxBatch)
	}
	if len(resp.Embeddings) != len(inputs) {
		t.Errorf("embeddings = %d, want %d", len(resp.Embeddings), len(inputs))
	}
}
