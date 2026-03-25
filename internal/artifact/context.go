package artifact

import "context"

type artifactsDirKey struct{}

// WithArtifactsDir returns a context carrying the artifacts directory path.
func WithArtifactsDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, artifactsDirKey{}, dir)
}

// ArtifactsDirFromContext extracts the artifacts directory from context.
func ArtifactsDirFromContext(ctx context.Context) string {
	dir, _ := ctx.Value(artifactsDirKey{}).(string)
	return dir
}
