package connector

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// TextChunkConnector implements text/chunk: split a long document into
// overlapping chunks for embedding. It composes with ai/embed (pass the chunks
// as `input`) and kb/upsert (pass them as `contents`): chunk → embed → upsert.
type TextChunkConnector struct{}

const (
	chunkDefaultSize    = 1000
	chunkDefaultOverlap = 0
)

// recursiveSeparators is the default hierarchy the "recursive" unit walks, from
// coarsest (paragraph) to finest. The empty string is the terminal fallback: a
// stretch with no separator left is hard-split by character window.
var recursiveSeparators = []string{"\n\n", "\n", ". ", " ", ""}

// chunkText splits text into overlapping chunks. `unit` selects the strategy:
//   - "chars" (default, Unicode-aware) / "words": fixed-size sliding window,
//     size and overlap counted in that unit.
//   - "recursive": separator-aware split (paragraph → line → sentence → word →
//     character) that prefers to break on natural boundaries, then merges
//     adjacent pieces toward `size` with `overlap` characters carried between
//     chunks. A chunk targets `size` but may reach up to `size + overlap`.
//
// Pure — no I/O — so it is fully unit-testable.
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
	case "recursive":
		return recursiveChunks(text, size, overlap, recursiveSeparators), nil
	default:
		return nil, fmt.Errorf("unknown unit %q (use chars, words, or recursive)", unit)
	}
}

// recursiveChunks splits text on a hierarchy of separators so chunks break on
// natural boundaries, then greedily merges the pieces (with overlap) into
// windows of at most `size` characters. size/overlap are counted in runes.
func recursiveChunks(text string, size, overlap int, seps []string) []string {
	pieces := splitRecursive(text, size, seps)
	return mergeSplits(pieces, size, overlap)
}

// splitRecursive breaks text into atomic pieces, each at most `size` runes where
// possible, preferring the coarsest separator that appears. A piece still larger
// than `size` after exhausting separators is hard-split by character window.
// Separators are re-attached to the pieces so the text reconstructs faithfully.
func splitRecursive(text string, size int, seps []string) []string {
	if utf8.RuneCountInString(text) <= size {
		if text == "" {
			return nil
		}
		return []string{text}
	}
	if len(seps) == 0 {
		return hardSplit(text, size)
	}
	sep, rest := seps[0], seps[1:]
	if sep == "" {
		return hardSplit(text, size)
	}
	parts := strings.Split(text, sep)
	var out []string
	for i, p := range parts {
		piece := p
		if i < len(parts)-1 { // keep the separator that we split on
			piece += sep
		}
		if piece == "" {
			continue
		}
		if utf8.RuneCountInString(piece) <= size {
			out = append(out, piece)
		} else {
			out = append(out, splitRecursive(piece, size, rest)...)
		}
	}
	return out
}

// hardSplit chops text into consecutive windows of `size` runes (no overlap),
// used as the terminal fallback for a stretch with no usable separator.
func hardSplit(text string, size int) []string {
	runes := []rune(text)
	return sliceChunks(len(runes), size, size, func(a, b int) string {
		return string(runes[a:b])
	})
}

// mergeSplits greedily concatenates pieces into chunks that target `size` runes.
// When a chunk fills, it starts the next one with a tail of the previous pieces
// totalling up to `overlap` runes, so consecutive chunks share context. Because
// that carried tail is prepended before the next piece, a chunk may reach up to
// `size + overlap` runes (this mirrors how splitters like LangChain's treat
// chunk_size as a target rather than a hard cap).
func mergeSplits(pieces []string, size, overlap int) []string {
	var chunks []string
	var cur []string
	curLen := 0

	flush := func() {
		if len(cur) == 0 {
			return
		}
		if c := strings.TrimSpace(strings.Join(cur, "")); c != "" {
			chunks = append(chunks, c)
		}
	}

	for _, p := range pieces {
		pl := utf8.RuneCountInString(p)
		if curLen+pl > size && len(cur) > 0 {
			flush()
			// Retain a trailing window of pieces up to `overlap` runes as the
			// start of the next chunk.
			for curLen > overlap && len(cur) > 0 {
				curLen -= utf8.RuneCountInString(cur[0])
				cur = cur[1:]
			}
		}
		cur = append(cur, p)
		curLen += pl
	}
	flush()
	return chunks
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
