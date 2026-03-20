package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PostgresEmitter persists audit events to an append-only Postgres table.
// No UPDATE or DELETE operations are provided to preserve immutability.
type PostgresEmitter struct {
	DB *sql.DB
}

func (p *PostgresEmitter) Emit(ctx context.Context, event Event) error {
	// Enrich event metadata with auth context if available.
	event = enrichFromContext(ctx, event)
	beforeJSON, err := marshalNullableJSON(event.Before)
	if err != nil {
		return fmt.Errorf("marshaling before state: %w", err)
	}

	afterJSON, err := marshalNullableJSON(event.After)
	if err != nil {
		return fmt.Errorf("marshaling after state: %w", err)
	}

	var metadataJSON []byte
	if event.Metadata != nil {
		metadataJSON, err = json.Marshal(event.Metadata)
		if err != nil {
			return fmt.Errorf("marshaling metadata: %w", err)
		}
	}

	ts := event.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	_, err = p.DB.ExecContext(ctx,
		`INSERT INTO audit_events (timestamp, actor, action, resource_type, resource_id, before_state, after_state, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ts, event.Actor, string(event.Action), event.Resource.Type, event.Resource.ID,
		nullableBytes(beforeJSON), nullableBytes(afterJSON), nullableBytes(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("inserting audit event: %w", err)
	}
	return nil
}

// QueryParams holds filter parameters for querying audit events.
type QueryParams struct {
	Action       string
	Actor        string
	ResourceType string
	ResourceID   string
	Since        time.Time
	Limit        int
}

// QueryRow represents a single audit event row returned from a query.
type QueryRow struct {
	ID           string
	Timestamp    time.Time
	Actor        string
	Action       string
	ResourceType string
	ResourceID   string
}

// Query retrieves audit events from the database matching the given filters.
func Query(ctx context.Context, db *sql.DB, params QueryParams) ([]QueryRow, error) {
	query := `SELECT id, timestamp, actor, action, resource_type, resource_id
		FROM audit_events WHERE 1=1`
	args := []any{}
	argIdx := 1

	if params.Action != "" {
		query += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, params.Action)
		argIdx++
	}
	if params.Actor != "" {
		query += fmt.Sprintf(" AND actor = $%d", argIdx)
		args = append(args, params.Actor)
		argIdx++
	}
	if params.ResourceType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", argIdx)
		args = append(args, params.ResourceType)
		argIdx++
	}
	if params.ResourceID != "" {
		query += fmt.Sprintf(" AND resource_id = $%d", argIdx)
		args = append(args, params.ResourceID)
		argIdx++
	}
	if !params.Since.IsZero() {
		query += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		args = append(args, params.Since)
		argIdx++
	}

	query += " ORDER BY timestamp DESC"

	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying audit events: %w", err)
	}
	defer rows.Close()

	var results []QueryRow
	for rows.Next() {
		var row QueryRow
		if err := rows.Scan(&row.ID, &row.Timestamp, &row.Actor, &row.Action, &row.ResourceType, &row.ResourceID); err != nil {
			return nil, fmt.Errorf("scanning audit event: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// marshalNullableJSON marshals a value to JSON, returning nil if the value is nil.
func marshalNullableJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// nullableBytes converts a byte slice to a sql-friendly nullable value.
func nullableBytes(b []byte) any {
	if b == nil {
		return nil
	}
	return b
}
