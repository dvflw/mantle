package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

func newProvider(database *sql.DB) (*goose.Provider, error) {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("creating sub filesystem: %w", err)
	}
	return goose.NewProvider(goose.DialectPostgres, database, fsys)
}

// Migrate runs all pending migrations.
func Migrate(ctx context.Context, database *sql.DB) error {
	provider, err := newProvider(database)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}
	_, err = provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

// MigrateDown rolls back the last applied migration.
func MigrateDown(ctx context.Context, database *sql.DB) error {
	provider, err := newProvider(database)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}
	_, err = provider.Down(ctx)
	if err != nil {
		return fmt.Errorf("rolling back migration: %w", err)
	}
	return nil
}

// MigrateStatus returns migration status as formatted text.
func MigrateStatus(ctx context.Context, database *sql.DB) (string, error) {
	provider, err := newProvider(database)
	if err != nil {
		return "", fmt.Errorf("creating migration provider: %w", err)
	}
	results, err := provider.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("getting migration status: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-30s  %s\n", "Applied At", "Migration")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("=", 60))
	for _, r := range results {
		if r.State == goose.StateApplied {
			fmt.Fprintf(&b, "%-30s  %s\n", r.AppliedAt.Format("2006-01-02 15:04:05 -0700"), r.Source.Path)
		} else {
			fmt.Fprintf(&b, "%-30s  %s\n", "Pending", r.Source.Path)
		}
	}
	return b.String(), nil
}
