package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// PostgresQueryConnector executes parameterized SQL queries against external
// Postgres databases. It connects per-execution and closes the connection
// afterward (stateless per step).
type PostgresQueryConnector struct{}

func (c *PostgresQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	// Extract connection URL from credential.
	connURL, err := extractPostgresURL(params)
	if err != nil {
		return nil, err
	}
	delete(params, "_credential")

	// Require the query parameter.
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("postgres/query: query is required")
	}

	// Build args slice from the optional args parameter.
	var args []any
	if rawArgs, ok := params["args"]; ok {
		switch v := rawArgs.(type) {
		case []any:
			args = v
		case []string:
			for _, s := range v {
				args = append(args, s)
			}
		default:
			return nil, fmt.Errorf("postgres/query: args must be an array, got %T", rawArgs)
		}
	}

	// Connect to the external database.
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return nil, fmt.Errorf("postgres/query: connecting: %w", err)
	}
	defer conn.Close(ctx)

	// Determine statement type to decide how to execute.
	if isSelectQuery(query) {
		return executeSelect(ctx, conn, query, args)
	}
	return executeExec(ctx, conn, query, args)
}

// extractPostgresURL pulls the database URL from the _credential map.
// Accepts {"url": "postgres://..."} or {"key": "postgres://..."}.
func extractPostgresURL(params map[string]any) (string, error) {
	cred, ok := params["_credential"].(map[string]string)
	if !ok {
		return "", fmt.Errorf("postgres/query: _credential is required (must contain url or key)")
	}

	if u := cred["url"]; u != "" {
		return u, nil
	}
	if k := cred["key"]; k != "" {
		return k, nil
	}
	return "", fmt.Errorf("postgres/query: credential must contain \"url\" or \"key\" field")
}

// isSelectQuery returns true if the trimmed, uppercased query starts with
// SELECT, WITH, or other read-only prefixes.
func isSelectQuery(query string) bool {
	upper := strings.TrimSpace(strings.ToUpper(query))
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

// executeSelect runs a query and returns all rows as maps.
func executeSelect(ctx context.Context, conn *pgx.Conn, query string, args []any) (map[string]any, error) {
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres/query: executing query: %w", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	colNames := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		colNames[i] = fd.Name
	}

	var result []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("postgres/query: scanning row: %w", err)
		}
		row := make(map[string]any, len(colNames))
		for i, col := range colNames {
			row[col] = values[i]
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres/query: reading rows: %w", err)
	}

	if result == nil {
		result = []map[string]any{}
	}

	return map[string]any{
		"rows":      result,
		"row_count": int64(len(result)),
	}, nil
}

// executeExec runs an INSERT/UPDATE/DELETE and returns rows affected.
func executeExec(ctx context.Context, conn *pgx.Conn, query string, args []any) (map[string]any, error) {
	tag, err := conn.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres/query: executing statement: %w", err)
	}

	return map[string]any{
		"rows_affected": tag.RowsAffected(),
	}, nil
}
