package connector

import (
	"context"
	"fmt"
	"strings"
)

// TextChunkConnector implements text/chunk: split a long document into
// overlapping chunks for embedding. It composes with ai/embed (pass the chunks
// as `input`) and kb/upsert (pass them as `contents`): chunk → embed → upsert.
type TextChunkConnector struct{}

const (
	chunkDefaultSize    = 1000
	chunkDefaultOverlap = 0
)

// chunkText splits text into fixed-size, optionally overlapping chunks. `unit`
// is "chars" (default, Unicode-aware) or "words" (whitespace-separated). size
// and overlap are counted in that unit. Pure — no I/O — so it is fully
// unit-testable.
func chunkText(text string, size, overlap int, unit string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text must not be empty")
	}
	if size <= 0 {
		return nil, fmt.Errorf("chunk_size must be positive, got %d", size)
	}
	if overlap < 0 {
		return nil, fmt.Errorf("chunk_overlap must be >= 0, got %d", overlap)
	}
	if overlap >= size {
		return nil, fmt.Errorf("chunk_overlap (%d) must be less than chunk_size (%d)", overlap, size)
	}
	step := size - overlap

	switch unit {
	case "", "chars":
		runes := []rune(text)
		return sliceChunks(len(runes), size, step, func(a, b int) string {
			return string(runes[a:b])
		}), nil
	case "words":
		words := strings.Fields(text)
		return sliceChunks(len(words), size, step, func(a, b int) string {
			return strings.Join(words[a:b], " ")
		}), nil
	default:
		return nil, fmt.Errorf("unknown unit %q (use chars or words)", unit)
	}
}

// sliceChunks walks [0,n) in windows of `size` advancing by `step`, emitting
// each window via slice(start, end). It stops once a window reaches the end so
// overlap never produces a trailing duplicate.
func sliceChunks(n, size, step int, slice func(a, b int) string) []string {
	var chunks []string
	for start := 0; start < n; start += step {
		end := start + size
		if end >= n {
			end = n
			chunks = append(chunks, slice(start, end))
			break
		}
		chunks = append(chunks, slice(start, end))
	}
	return chunks
}

func (c *TextChunkConnector) Execute(_ context.Context, params map[string]any) (map[string]any, error) {
	text, _ := params["text"].(string)

	size := chunkDefaultSize
	if v, ok := extractInt(params["chunk_size"]); ok {
		size = v
	}
	overlap := chunkDefaultOverlap
	if v, ok := extractInt(params["chunk_overlap"]); ok {
		overlap = v
	}
	unit, _ := params["unit"].(string)

	chunks, err := chunkText(text, size, overlap, unit)
	if err != nil {
		return nil, fmt.Errorf("text/chunk: %w", err)
	}

	return map[string]any{
		"chunks": chunks,
		"count":  len(chunks),
	}, nil
}
