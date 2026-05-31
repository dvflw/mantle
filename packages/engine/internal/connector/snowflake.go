package connector

import (
	"context"
	"database/sql"
	"fmt"

	sf "github.com/snowflakedb/gosnowflake" // named import registers "snowflake" driver via init()
)

// extractSnowflakeCredential pulls Snowflake connection fields from the _credential param.
// Schema defaults to "PUBLIC" if empty.
// Deletes _credential from params.
func extractSnowflakeCredential(params map[string]any) (account, user, password, database, schema, warehouse string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", "", "", "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	var cred map[string]string
	switch v := raw.(type) {
	case map[string]string:
		cred = v
	case map[string]any:
		cred = make(map[string]string, len(v))
		for k, val := range v {
			if s, ok := val.(string); ok {
				cred[k] = s
			}
		}
	default:
		return "", "", "", "", "", "", fmt.Errorf("credential is required")
	}

	account = cred["account"]
	if account == "" {
		return "", "", "", "", "", "", fmt.Errorf("credential must contain an 'account' field")
	}
	user = cred["user"]
	if user == "" {
		return "", "", "", "", "", "", fmt.Errorf("credential must contain a 'user' field")
	}
	password = cred["password"]
	if password == "" {
		return "", "", "", "", "", "", fmt.Errorf("credential must contain a 'password' field")
	}
	database = cred["database"]
	schema = cred["schema"]
	if schema == "" {
		schema = "PUBLIC"
	}
	warehouse = cred["warehouse"]
	return account, user, password, database, schema, warehouse, nil
}

// SnowflakeQueryConnector executes a SQL query against Snowflake.
// Params: query (required), max_rows (optional, default 1000).
// Output: {"rows": [...], "count": N}
type SnowflakeQueryConnector struct{}

func (c *SnowflakeQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("snowflake/query: query is required")
	}

	maxRows := 1000
	if m, ok := extractInt(params["max_rows"]); ok && m > 0 {
		maxRows = m
	}

	var args []any
	if rawArgs, ok := params["args"].([]any); ok {
		args = rawArgs
	}

	account, user, password, database, schema, warehouse, err := extractSnowflakeCredential(params)
	if err != nil {
		return nil, fmt.Errorf("snowflake/query: %w", err)
	}

	dsn, err := sf.DSN(&sf.Config{
		Account:   account,
		User:      user,
		Password:  password,
		Database:  database,
		Schema:    schema,
		Warehouse: warehouse,
	})
	if err != nil {
		return nil, fmt.Errorf("snowflake/query: building DSN: %w", err)
	}

	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return nil, fmt.Errorf("snowflake/query: opening connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("snowflake/query: connecting: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("snowflake/query: executing query: %w", err)
	}
	defer rows.Close()

	result, err := scanSQLRows(rows, maxRows)
	if err != nil {
		return nil, fmt.Errorf("snowflake/query: scanning rows: %w", err)
	}

	if result == nil {
		result = []map[string]any{}
	}
	return map[string]any{
		"rows":  result,
		"count": len(result),
	}, nil
}
