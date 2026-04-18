package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiscoveredFile is one YAML file found by Discover.
type DiscoveredFile struct {
	RelPath string // path relative to the scan root (BasePath + Path)
	Bytes   []byte // file contents
	Hash    string // sha256 hex of Bytes
}

// Discover walks scanRoot/subPath and returns every .yaml / .yml file
// inside it (recursively). Files outside subPath are ignored. The scan
// is single-threaded and reads each file into memory — workflow YAML
// files are small and the total repo size is bounded by the sync
// engine's upstream guardrails.
//
// Returns an error when subPath doesn't resolve to a directory under
// scanRoot so callers can distinguish "repo layout changed" from "no
// workflows found."
func Discover(scanRoot, subPath string) ([]DiscoveredFile, error) {
	absRoot, err := filepath.Abs(scanRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving scan root: %w", err)
	}
	target := filepath.Join(absRoot, filepath.FromSlash(strings.TrimPrefix(subPath, "/")))
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", target, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", target)
	}

	var out []DiscoveredFile
	err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !isYAML(d.Name()) {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", path, readErr)
		}
		rel, relErr := filepath.Rel(target, path)
		if relErr != nil {
			return fmt.Errorf("computing relpath for %s: %w", path, relErr)
		}
		h := sha256.Sum256(b)
		out = append(out, DiscoveredFile{
			RelPath: rel,
			Bytes:   b,
			Hash:    hex.EncodeToString(h[:]),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func isYAML(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
