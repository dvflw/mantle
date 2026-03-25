package artifact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// TmpStorage persists and retrieves artifact files.
type TmpStorage interface {
	// Put copies a local file to tmp storage at the given key.
	// Returns the URL or path where the file can be accessed.
	Put(ctx context.Context, key string, localPath string) (string, error)

	// DeleteByPrefix removes all files under the given key prefix.
	DeleteByPrefix(ctx context.Context, prefix string) error
}

// FilesystemTmpStorage stores artifacts on the local filesystem.
type FilesystemTmpStorage struct {
	BasePath string
}

func (fs *FilesystemTmpStorage) Put(ctx context.Context, key string, localPath string) (string, error) {
	destPath := filepath.Join(fs.BasePath, key)
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", fmt.Errorf("creating artifact dir: %w", err)
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
		return "", fmt.Errorf("copying artifact: %w", err)
	}

	return destPath, nil
}

func (fs *FilesystemTmpStorage) DeleteByPrefix(ctx context.Context, prefix string) error {
	target := filepath.Join(fs.BasePath, prefix)
	absBase, err := filepath.Abs(fs.BasePath)
	if err != nil {
		return fmt.Errorf("resolving base path: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolving target path: %w", err)
	}
	if !strings.HasPrefix(absTarget, absBase) {
		return fmt.Errorf("prefix escapes base path")
	}
	return os.RemoveAll(target)
}
