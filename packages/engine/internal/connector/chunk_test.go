package connector

import (
	"context"
	"strings"
	"testing"
)

func TestChunkText_Chars(t *testing.T) {
	// "abcdefg" (7), size 3, overlap 1 → step 2: windows 0-3, 2-5, 4-7.
	// The last window reaches the end, so there is no trailing "g" chunk.
	got, err := chunkText("abcdefg", 3, 1, "chars")
	if err != nil {
		t.Fatalf("chunkText error: %v", err)
	}
	want := []string{"abc", "cde", "efg"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("chunks = %v, want %v", got, want)
	}
}

func TestChunkText_NoOverlapExactFit(t *testing.T) {
	// 6 chars, size 3, overlap 0 → [abc, def], no trailing empty chunk.
	got, err := chunkText("abcdef", 3, 0, "chars")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 2 || got[0] != "abc" || got[1] != "def" {
		t.Errorf("chunks = %v, want [abc def]", got)
	}
}

func TestChunkText_Words(t *testing.T) {
	got, err := chunkText("the quick brown fox jumps", 2, 0, "words")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []string{"the quick", "brown fox", "jumps"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("chunks = %v, want %v", got, want)
	}
}

func TestChunkText_ShorterThanSize(t *testing.T) {
	got, err := chunkText("hi", 100, 10, "chars")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 || got[0] != "hi" {
		t.Errorf("chunks = %v, want [hi]", got)
	}
}

func TestChunkText_Unicode(t *testing.T) {
	// Multibyte runes must not be split mid-character.
	got, err := chunkText("héllo wörld", 3, 0, "chars")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// 11 runes / size 3 → 4 chunks; first is "hél".
	if got[0] != "hél" {
		t.Errorf("first chunk = %q, want %q", got[0], "hél")
	}
}

func TestChunkText_RecursiveSentences(t *testing.T) {
	// With ". " available and a size that holds one sentence but not two, the
	// recursive splitter breaks on sentence boundaries.
	got, err := chunkText("Alpha beta. Gamma delta. Epsilon zeta.", 15, 0, "recursive")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []string{"Alpha beta.", "Gamma delta.", "Epsilon zeta."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("chunks = %v, want %v", got, want)
	}
}

func TestChunkText_RecursiveFallbackHardSplit(t *testing.T) {
	// No usable separator (one long token): falls through to a character window.
	got, err := chunkText("abcdefgh", 3, 0, "recursive")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := []string{"abc", "def", "gh"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("chunks = %v, want %v", got, want)
	}
}

func TestChunkText_RecursiveOverlap(t *testing.T) {
	// Overlap carries a trailing word into the next chunk, so adjacent chunks
	// share the boundary token.
	got, err := chunkText("one two three four five six", 9, 4, "recursive")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) < 2 || got[0] != "one two" || got[1] != "two three" {
		t.Errorf("chunks = %v, want first two [one two, two three]", got)
	}
}

func TestChunkText_RecursiveShorterThanSize(t *testing.T) {
	got, err := chunkText("short text", 100, 10, "recursive")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 1 || got[0] != "short text" {
		t.Errorf("chunks = %v, want [short text]", got)
	}
}

func TestChunkText_RecursiveChunkSizeBound(t *testing.T) {
	// Every chunk stays within size + overlap runes (the documented bound).
	text := strings.Repeat("lorem ipsum dolor sit amet. ", 50)
	size, overlap := 60, 15
	got, err := chunkText(text, size, overlap, "recursive")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	for i, c := range got {
		if n := len([]rune(c)); n > size+overlap {
			t.Errorf("chunk %d has %d runes, exceeds size+overlap (%d)", i, n, size+overlap)
		}
	}
}

func TestChunkText_Errors(t *testing.T) {
	cases := []struct {
		name          string
		text          string
		size, overlap int
		unit          string
	}{
		{"empty text", "   ", 10, 0, "chars"},
		{"zero size", "abc", 0, 0, "chars"},
		{"negative overlap", "abc", 10, -1, "chars"},
		{"overlap >= size", "abc", 5, 5, "chars"},
		{"unknown unit", "abc", 5, 0, "tokens"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := chunkText(tc.text, tc.size, tc.overlap, tc.unit); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestTextChunkConnector_Output(t *testing.T) {
	out, err := (&TextChunkConnector{}).Execute(context.Background(), map[string]any{
		"text":          "one two three four five",
		"chunk_size":    2,
		"chunk_overlap": 0,
		"unit":          "words",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	chunks := out["chunks"].([]string)
	if out["count"].(int) != len(chunks) || len(chunks) != 3 {
		t.Errorf("count/chunks = %v / %v", out["count"], chunks)
	}
}
