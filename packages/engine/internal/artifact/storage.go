package artifact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Storage persists and retrieves artifact files.
type Storage interface {
	// Put copies a local file to storage at the given key.
	// Returns the URL or path where the file can be accessed.
	Put(ctx context.Context, key string, localPath string) (string, error)

	// Delete removes a single artifact by the URL/path returned from Put.
	Delete(ctx context.Context, url string) error

	// DeleteByPrefix removes all files under the given key prefix.
	DeleteByPrefix(ctx context.Context, prefix string) error
}

// FilesystemStorage stores artifacts on the local filesystem.
type FilesystemStorage struct {
	BasePath string
}

// Put copies the file at localPath to the storage location identified by key and returns its path.
func (fs *FilesystemStorage) Put(ctx context.Context, key string, localPath string) (string, error) {
	destPath := filepath.Join(fs.BasePath, key)
	// Validate destination is within BasePath to prevent path traversal.
	// Use EvalSymlinks for canonicalization to prevent symlink-based escapes.
	resolvedBase, err := filepath.EvalSymlinks(fs.BasePath)
	if err != nil {
		return "", fmt.Errorf("resolving base path: %w", err)
	}
	// Ensure dest directory exists before resolving symlinks on the full path.
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", fmt.Errorf("creating artifact dir: %w", err)
	}
	resolvedDest, err := filepath.EvalSymlinks(filepath.Dir(destPath))
	if err != nil {
		return "", fmt.Errorf("resolving dest path: %w", err)
	}
	resolvedDest = filepath.Join(resolvedDest, filepath.Base(destPath))
	rel, err := filepath.Rel(resolvedBase, resolvedDest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("key escapes base path")
	}

	// Reject symlinks and non-regular files to prevent exfiltration.
	fi, err := os.Lstat(localPath)
	if err != nil {
		return "", fmt.Errorf("stat source file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("source file %q is not a regular file (mode: %s)", localPath, fi.Mode())
	}

	src, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("opening source file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("creating dest file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(destPath)
		return "", fmt.Errorf("copying artifact: %w", err)
	}

	return destPath, nil
}

// DeleteByPrefix removes all files stored under the given key prefix.
func (fs *FilesystemStorage) DeleteByPrefix(ctx context.Context, prefix string) error {
	target := filepath.Join(fs.BasePath, prefix)
	resolvedBase, err := filepath.EvalSymlinks(fs.BasePath)
	if err != nil {
		return fmt.Errorf("resolving base path: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		// Target doesn't exist — fall back to Abs for traversal check.
		absTarget, absErr := filepath.Abs(target)
		if absErr != nil {
			return fmt.Errorf("resolving target path: %w", err)
		}
		rel, relErr := filepath.Rel(resolvedBase, absTarget)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("prefix escapes base path")
		}
		// Target doesn't exist and is within base — nothing to delete.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("resolving target path: %w", err)
	}
	rel, err := filepath.Rel(resolvedBase, resolvedTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("prefix escapes base path")
	}
	return os.RemoveAll(resolvedTarget)
}

// Delete removes a single artifact file by its path (as returned by Put).
func (fs *FilesystemStorage) Delete(ctx context.Context, url string) error {
	resolvedBase, err := filepath.EvalSymlinks(fs.BasePath)
	if err != nil {
		return fmt.Errorf("resolving base path: %w", err)
	}
	resolvedURL, err := filepath.EvalSymlinks(url)
	if err != nil {
		// File doesn't exist — fall back to Abs for traversal check.
		absURL, absErr := filepath.Abs(url)
		if absErr != nil {
			return fmt.Errorf("resolving artifact path: %w", err)
		}
		rel, relErr := filepath.Rel(resolvedBase, absURL)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("artifact path escapes base path")
		}
		if os.IsNotExist(err) {
			return nil // already gone
		}
		return fmt.Errorf("resolving artifact path: %w", err)
	}
	rel, err := filepath.Rel(resolvedBase, resolvedURL)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("artifact path escapes base path")
	}
	if err := os.Remove(resolvedURL); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
