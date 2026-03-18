package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseResult holds the parsed workflow and the raw YAML node tree for line number lookups.
type ParseResult struct {
	Workflow *Workflow
	Root     *yaml.Node
}

// Parse reads a YAML workflow file and returns the parsed workflow with the
// raw YAML node tree preserved for line/column error reporting.
//
// It performs two-pass parsing:
//  1. Unmarshal into yaml.Node to preserve line/column information
//  2. Decode the node into the Workflow struct
func Parse(filename string) (*ParseResult, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading workflow file: %w", err)
	}

	// Pass 1: parse into yaml.Node to preserve source positions.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	// The top-level node is a document node; the actual mapping is its first child.
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("parsing YAML: expected document node")
	}
	root := doc.Content[0]

	// Pass 2: decode the node into the Workflow struct.
	var w Workflow
	if err := root.Decode(&w); err != nil {
		return nil, fmt.Errorf("decoding workflow: %w", err)
	}

	return &ParseResult{
		Workflow: &w,
		Root:     root,
	}, nil
}
