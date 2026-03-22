package db

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"

	"github.com/dvflw/mantle/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type contextKey struct{}

// Open connects to the database using the provided configuration and applies
// connection pool settings. If pool fields are zero-valued, database/sql
// defaults are left in place (callers such as tests that only need a URL can
// pass a DatabaseConfig with just the URL field set).
func Open(cfg config.DatabaseConfig) (*sql.DB, error) {
	if strings.Contains(cfg.URL, "sslmode=disable") {
		slog.Warn("database connection using sslmode=disable — use sslmode=require or sslmode=verify-full for production")
	}

	database, err := sql.Open("pgx", cfg.URL)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		database.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		database.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		database.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if err := database.Ping(); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func WithContext(ctx context.Context, database *sql.DB) context.Context {
	return context.WithValue(ctx, contextKey{}, database)
}

func FromContext(ctx context.Context) *sql.DB {
	database, _ := ctx.Value(contextKey{}).(*sql.DB)
	return database
}
