package db

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type contextKey struct{}

func Open(databaseURL string) (*sql.DB, error) {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
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
