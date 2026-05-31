package connector

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/go-sql-driver/mysql"         // register "mysql" driver
	_ "github.com/jackc/pgx/v5/stdlib"         // register "pgx" driver for Redshift
	_ "github.com/microsoft/go-mssqldb"        // register "sqlserver" driver
)

// ---- MySQL ----

// extractMySQLCredential builds a MySQL DSN from the _credential param.
// Credential: {host, port, user, password, database}.
// Deletes _credential from params.
func extractMySQLCredential(params map[string]any) (dsn string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", fmt.Errorf("credential is required")
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
		return "", fmt.Errorf("credential is required")
	}

	host := cred["host"]
	if host == "" {
		return "", fmt.Errorf("credential must contain a 'host' field")
	}
	port := cred["port"]
	if port == "" {
		port = "3306"
	}
	user := cred["user"]
	if user == "" {
		return "", fmt.Errorf("credential must contain a 'user' field")
	}
	password := cred["password"]
	database := cred["database"]
	if database == "" {
		return "", fmt.Errorf("credential must contain a 'database' field")
	}

	dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, password, host, port, database)
	return dsn, nil
}

// scanSQLRows scans database/sql rows into a slice of maps.
func scanSQLRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// MySQLQueryConnector executes a SELECT query against MySQL.
// Params: query (required), max_rows (optional, default 1000).
// Output: {"rows": [...], "count": N}
type MySQLQueryConnector struct{}

func (c *MySQLQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("mysql/query: query is required")
	}

	maxRows := 1000
	if m, ok := extractInt(params["max_rows"]); ok && m > 0 {
		maxRows = m
	}

	dsn, err := extractMySQLCredential(params)
	if err != nil {
		return nil, fmt.Errorf("mysql/query: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql/query: opening connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mysql/query: connecting: %w", err)
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mysql/query: executing query: %w", err)
	}
	defer rows.Close()

	result, err := scanSQLRows(rows)
	if err != nil {
		return nil, fmt.Errorf("mysql/query: scanning rows: %w", err)
	}

	if len(result) > maxRows {
		result = result[:maxRows]
	}
	if result == nil {
		result = []map[string]any{}
	}
	return map[string]any{
		"rows":  result,
		"count": len(result),
	}, nil
}

// MySQLExecuteConnector executes a non-SELECT statement against MySQL.
// Params: statement (required).
// Output: {"rows_affected": N, "last_insert_id": N}
type MySQLExecuteConnector struct{}

func (c *MySQLExecuteConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	statement, _ := params["statement"].(string)
	if statement == "" {
		return nil, fmt.Errorf("mysql/execute: statement is required")
	}

	dsn, err := extractMySQLCredential(params)
	if err != nil {
		return nil, fmt.Errorf("mysql/execute: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql/execute: opening connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mysql/execute: connecting: %w", err)
	}

	res, err := db.ExecContext(ctx, statement)
	if err != nil {
		return nil, fmt.Errorf("mysql/execute: executing statement: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	lastInsertID, _ := res.LastInsertId()

	return map[string]any{
		"rows_affected":  rowsAffected,
		"last_insert_id": lastInsertID,
	}, nil
}

// ---- MSSQL ----

// extractMSSQLCredential builds a SQL Server connection string from the _credential param.
// Credential: {host, port, user, password, database}.
// Deletes _credential from params.
func extractMSSQLCredential(params map[string]any) (dsn string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", fmt.Errorf("credential is required")
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
		return "", fmt.Errorf("credential is required")
	}

	host := cred["host"]
	if host == "" {
		return "", fmt.Errorf("credential must contain a 'host' field")
	}
	port := cred["port"]
	if port == "" {
		port = "1433"
	}
	user := cred["user"]
	if user == "" {
		return "", fmt.Errorf("credential must contain a 'user' field")
	}
	password := cred["password"]
	database := cred["database"]
	if database == "" {
		return "", fmt.Errorf("credential must contain a 'database' field")
	}

	u := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(user, password),
		Host:   host + ":" + port,
	}
	q := u.Query()
	q.Set("database", database)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// MSSQLQueryConnector executes a SELECT query against SQL Server.
// Params: query (required), max_rows (optional, default 1000).
// Output: {"rows": [...], "count": N}
type MSSQLQueryConnector struct{}

func (c *MSSQLQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("mssql/query: query is required")
	}

	maxRows := 1000
	if m, ok := extractInt(params["max_rows"]); ok && m > 0 {
		maxRows = m
	}

	dsn, err := extractMSSQLCredential(params)
	if err != nil {
		return nil, fmt.Errorf("mssql/query: %w", err)
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("mssql/query: opening connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mssql/query: connecting: %w", err)
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mssql/query: executing query: %w", err)
	}
	defer rows.Close()

	result, err := scanSQLRows(rows)
	if err != nil {
		return nil, fmt.Errorf("mssql/query: scanning rows: %w", err)
	}

	if len(result) > maxRows {
		result = result[:maxRows]
	}
	if result == nil {
		result = []map[string]any{}
	}
	return map[string]any{
		"rows":  result,
		"count": len(result),
	}, nil
}

// MSSQLExecuteConnector executes a non-SELECT statement against SQL Server.
// Params: statement (required).
// Output: {"rows_affected": N, "last_insert_id": N}
type MSSQLExecuteConnector struct{}

func (c *MSSQLExecuteConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	statement, _ := params["statement"].(string)
	if statement == "" {
		return nil, fmt.Errorf("mssql/execute: statement is required")
	}

	dsn, err := extractMSSQLCredential(params)
	if err != nil {
		return nil, fmt.Errorf("mssql/execute: %w", err)
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("mssql/execute: opening connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mssql/execute: connecting: %w", err)
	}

	res, err := db.ExecContext(ctx, statement)
	if err != nil {
		return nil, fmt.Errorf("mssql/execute: executing statement: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	lastInsertID, _ := res.LastInsertId()

	return map[string]any{
		"rows_affected":  rowsAffected,
		"last_insert_id": lastInsertID,
	}, nil
}

// ---- Redshift ----

// extractRedshiftCredential builds a pgx connection string for Redshift.
// Credential: {host, port, user, password, database}.
// Deletes _credential from params.
func extractRedshiftCredential(params map[string]any) (connStr string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", fmt.Errorf("credential is required")
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
		return "", fmt.Errorf("credential is required")
	}

	host := cred["host"]
	if host == "" {
		return "", fmt.Errorf("credential must contain a 'host' field")
	}
	port := cred["port"]
	if port == "" {
		port = "5439"
	}
	user := cred["user"]
	if user == "" {
		return "", fmt.Errorf("credential must contain a 'user' field")
	}
	password := cred["password"]
	database := cred["database"]
	if database == "" {
		return "", fmt.Errorf("credential must contain a 'database' field")
	}

	connStr = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		host, port, user, password, database)
	return connStr, nil
}

// RedshiftQueryConnector executes a SQL query against Amazon Redshift (Postgres-compatible).
// Params: query (required), max_rows (optional, default 1000).
// Output: {"rows": [...], "count": N}
type RedshiftQueryConnector struct{}

func (c *RedshiftQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("redshift/query: query is required")
	}

	maxRows := 1000
	if m, ok := extractInt(params["max_rows"]); ok && m > 0 {
		maxRows = m
	}

	connStr, err := extractRedshiftCredential(params)
	if err != nil {
		return nil, fmt.Errorf("redshift/query: %w", err)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("redshift/query: opening connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("redshift/query: connecting: %w", err)
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("redshift/query: executing query: %w", err)
	}
	defer rows.Close()

	result, err := scanSQLRows(rows)
	if err != nil {
		return nil, fmt.Errorf("redshift/query: scanning rows: %w", err)
	}

	if len(result) > maxRows {
		result = result[:maxRows]
	}
	if result == nil {
		result = []map[string]any{}
	}
	return map[string]any{
		"rows":  result,
		"count": len(result),
	}, nil
}
