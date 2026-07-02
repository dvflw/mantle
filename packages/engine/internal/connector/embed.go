package connector

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/dvflw/mantle/internal/metrics"
)

// EmbeddingConnector implements the ai/embed action: it turns text into vector
// embeddings via a provider's embeddings API. It mirrors AIConnector's provider
// selection, base-URL/model allowlists, AWS config, and metrics. Because the
// action lives under the "ai/" prefix, the engine's token-budget accounting
// applies to it automatically (via the usage.total_tokens it returns).
type EmbeddingConnector struct {
	Client          *http.Client
	AWSConfigFunc   func(ctx context.Context, cred map[string]string, defaultRegion string) (aws.Config, error)
	DefaultRegion   string
	AllowedBaseURLs []string
	AllowedModels   []string // empty = all models allowed
}

// Execute embeds one or more input strings and returns the vectors, including
// pgvector text literals for direct use in a postgres/query arg (e.g.
// INSERT ... VALUES ($1::vector)).
func (c *EmbeddingConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	providerName, _ := params["provider"].(string)
	if providerName == "" {
		providerName = "openai"
	}

	model, _ := params["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("ai/embed: model is required")
	}
	if len(c.AllowedModels) > 0 && !stringInSlice(model, c.AllowedModels) {
		return nil, fmt.Errorf("ai/embed: model %q not in allowed list", model)
	}

	inputs, err := extractEmbedInputs(params)
	if err != nil {
		return nil, fmt.Errorf("ai/embed: %w", err)
	}

	provider, err := c.getProvider(providerName, params)
	if err != nil {
		return nil, fmt.Errorf("ai/embed: %w", err)
	}

	req := &EmbeddingRequest{Model: model, Inputs: inputs}
	if cred, ok := params["_credential"].(map[string]string); ok {
		req.Credential = cred
	}
	if d, ok := extractInt(params["dimensions"]); ok && d > 0 {
		req.Dimensions = d
	}

	workflow, _ := params["_workflow"].(string)
	step, _ := params["_step"].(string)

	start := time.Now()
	resp, err := provider.Embeddings(ctx, req)
	duration := time.Since(start).Seconds()

	metrics.AIRequestDuration.WithLabelValues(workflow, step, model, providerName).Observe(duration)
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues(workflow, step, model, providerName, "error").Inc()
		return nil, fmt.Errorf("ai/embed: %w", err)
	}
	metrics.AITokensTotal.WithLabelValues(workflow, step, model, providerName, "prompt").Add(float64(resp.Usage.PromptTokens))
	metrics.AIRequestsTotal.WithLabelValues(workflow, step, model, providerName, "success").Inc()

	return embeddingResponseToOutput(resp), nil
}

// getProvider returns the EmbeddingProvider for the given provider name.
func (c *EmbeddingConnector) getProvider(name string, params map[string]any) (EmbeddingProvider, error) {
	switch name {
	case "openai":
		baseURL := "https://api.openai.com/v1"
		if u, ok := params["base_url"].(string); ok && u != "" {
			baseURL = u
		}
		if len(c.AllowedBaseURLs) > 0 && !stringInSlice(baseURL, c.AllowedBaseURLs) {
			return nil, fmt.Errorf("base_url %q not in allowed list", baseURL)
		}
		return &OpenAIProvider{Client: c.Client, BaseURL: baseURL}, nil
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
			return nil, fmt.Errorf("[bedrock]: %w", err)
		}
		return &BedrockEmbeddingProvider{Client: bedrockruntime.NewFromConfig(awsCfg)}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (available: openai, bedrock)", name)
	}
}

// extractEmbedInputs reads the `input` param, accepting a single string or an
// array of strings.
func extractEmbedInputs(params map[string]any) ([]string, error) {
	raw, ok := params["input"]
	if !ok {
		return nil, fmt.Errorf("input is required")
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("input must not be empty")
		}
		return []string{v}, nil
	case []string:
		if len(v) == 0 {
			return nil, fmt.Errorf("input must not be empty")
		}
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("input[%d] must be a string, got %T", i, item)
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("input must not be empty")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("input must be a string or array of strings, got %T", raw)
	}
}

// embeddingResponseToOutput converts an EmbeddingResponse to the connector's
// output map. It always includes the full `embeddings`/`vectors` arrays, and
// for the common single-input case surfaces `embedding`/`vector`/`dimensions`.
func embeddingResponseToOutput(resp *EmbeddingResponse) map[string]any {
	vectors := make([]string, len(resp.Embeddings))
	for i, e := range resp.Embeddings {
		vectors[i] = formatPGVector(e)
	}

	out := map[string]any{
		"model":      resp.Model,
		"embeddings": resp.Embeddings,
		"vectors":    vectors,
		"count":      len(resp.Embeddings),
		"usage": map[string]any{
			"prompt_tokens": resp.Usage.PromptTokens,
			"total_tokens":  resp.Usage.TotalTokens,
		},
	}
	if len(resp.Embeddings) > 0 {
		out["embedding"] = resp.Embeddings[0]
		out["vector"] = vectors[0]
		out["dimensions"] = len(resp.Embeddings[0])
	}
	return out
}

// formatPGVector renders a float slice as a pgvector text literal: "[1,2,3]".
// This can be passed straight into a postgres/query arg and cast with ::vector.
func formatPGVector(v []float64) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
	}
	b.WriteByte(']')
	return b.String()
}

func stringInSlice(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
