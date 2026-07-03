package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The kb/* connectors are thin pgvector helpers over a Postgres database (the
// step's `credential` is a postgres credential). They compose with ai/embed:
// take the pgvector literal from ai/embed's `output.vector` / `output.vectors`
// and store or query it, hiding the ::vector casts, distance operators, and
// multi-row unnest inserts. They do not manage schema — create the table
// yourself (see the RAG guide / rag-kb-schema.sql).

// kbIdentRe matches a safe SQL identifier, optionally schema-qualified. Table
// and column names are interpolated into SQL (they cannot be bound as
// parameters), so every identifier is validated against this before use.
var kbIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

const (
	kbDefaultContentColumn   = "content"
	kbDefaultEmbeddingColumn = "embedding"
	kbDefaultMetadataColumn  = "metadata"
	kbDefaultTopK            = 5
	kbMaxTopK                = 1000
	// kbConnectTimeout bounds the Postgres connection handshake so a stalled
	// server can't hang a step that has no timeout of its own. It only shortens
	// an existing deadline, never extends one.
	kbConnectTimeout = 15 * time.Second
)

// kbIdentParam reads an identifier param, applies a default when absent, and
// validates it to prevent SQL injection through table/column names.
func kbIdentParam(params map[string]any, key, def string) (string, error) {
	v, ok := params[key].(string)
	if !ok || v == "" {
		v = def
	}
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	if !kbIdentRe.MatchString(v) {
		return "", fmt.Errorf("%s %q is not a valid SQL identifier", key, v)
	}
	return v, nil
}

// kbStrings normalizes a "single or many" text param: `singleKey` (a string) or
// `manyKey` (an array of strings). Exactly one must be present.
func kbStrings(params map[string]any, singleKey, manyKey string) ([]string, error) {
	single, hasSingle := params[singleKey]
	many, hasMany := params[manyKey]
	if hasSingle == hasMany { // both or neither
		return nil, fmt.Errorf("exactly one of %s or %s is required", singleKey, manyKey)
	}
	if hasSingle {
		s, ok := single.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("%s must be a non-empty string", singleKey)
		}
		return []string{s}, nil
	}
	switch v := many.(type) {
	case []string:
		if len(v) == 0 {
			return nil, fmt.Errorf("%s must not be empty", manyKey)
		}
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be a string, got %T", manyKey, i, item)
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s must not be empty", manyKey)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings, got %T", manyKey, many)
	}
}

// kbMetadata normalizes the optional metadata param into JSON strings aligned
// with the rows. A single `metadata` object is broadcast to every row (handy
// for chunked ingest that shares a title/source); `metadatas` must match the
// row count one-to-one. Returns present=false when no metadata param is set.
func kbMetadata(params map[string]any, rows int) (jsons []string, present bool, err error) {
	single, hasSingle := params["metadata"]
	many, hasMany := params["metadatas"]
	if !hasSingle && !hasMany {
		return nil, false, nil
	}
	if hasSingle && hasMany {
		return nil, false, fmt.Errorf("set only one of metadata or metadatas")
	}

	if hasSingle {
		b, mErr := json.Marshal(single)
		if mErr != nil {
			return nil, false, fmt.Errorf("marshaling metadata: %w", mErr)
		}
		out := make([]string, rows)
		for i := range out {
			out[i] = string(b)
		}
		return out, true, nil
	}

	arr, ok := many.([]any)
	if !ok {
		return nil, false, fmt.Errorf("metadatas must be an array of objects, got %T", many)
	}
	if len(arr) != rows {
		return nil, false, fmt.Errorf("metadatas count %d does not match content count %d", len(arr), rows)
	}
	out := make([]string, len(arr))
	for i, o := range arr {
		b, mErr := json.Marshal(o)
		if mErr != nil {
			return nil, false, fmt.Errorf("marshaling metadatas[%d]: %w", i, mErr)
		}
		out[i] = string(b)
	}
	return out, true, nil
}

// prepareUpsert validates params and builds the INSERT ... SELECT unnest(...)
// statement plus its args. Pure (no DB) so it is fully unit-testable.
func prepareUpsert(params map[string]any) (string, []any, error) {
	table, err := kbIdentParam(params, "table", "")
	if err != nil {
		return "", nil, err
	}
	contentCol, err := kbIdentParam(params, "content_column", kbDefaultContentColumn)
	if err != nil {
		return "", nil, err
	}
	embCol, err := kbIdentParam(params, "embedding_column", kbDefaultEmbeddingColumn)
	if err != nil {
		return "", nil, err
	}
	metaCol, err := kbIdentParam(params, "metadata_column", kbDefaultMetadataColumn)
	if err != nil {
		return "", nil, err
	}

	contents, err := kbStrings(params, "content", "contents")
	if err != nil {
		return "", nil, err
	}
	vectors, err := kbStrings(params, "vector", "vectors")
	if err != nil {
		return "", nil, err
	}
	if len(vectors) != len(contents) {
		return "", nil, fmt.Errorf("vector count %d does not match content count %d", len(vectors), len(contents))
	}

	metas, hasMeta, err := kbMetadata(params, len(contents))
	if err != nil {
		return "", nil, err
	}

	var conflict string
	if ct, ok := params["conflict_target"].(string); ok && ct != "" {
		// Accept a single column or a comma-separated list (composite key),
		// validating each part as an identifier.
		parts := strings.Split(ct, ",")
		for i, p := range parts {
			p = strings.TrimSpace(p)
			if !kbIdentRe.MatchString(p) {
				return "", nil, fmt.Errorf("conflict_target %q is not a valid SQL identifier", ct)
			}
			parts[i] = p
		}
		conflict = fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", strings.Join(parts, ", "))
	}

	var sb strings.Builder
	args := []any{contents, vectors}
	if hasMeta {
		fmt.Fprintf(&sb,
			"INSERT INTO %s (%s, %s, %s) SELECT c, e::vector, m::jsonb FROM unnest($1::text[], $2::text[], $3::text[]) AS u(c, e, m)%s",
			table, contentCol, embCol, metaCol, conflict)
		args = append(args, metas)
	} else {
		fmt.Fprintf(&sb,
			"INSERT INTO %s (%s, %s) SELECT c, e::vector FROM unnest($1::text[], $2::text[]) AS u(c, e)%s",
			table, contentCol, embCol, conflict)
	}
	return sb.String(), args, nil
}

// kbDistanceOperator maps a metric name to its pgvector operator.
func kbDistanceOperator(metric string) (string, error) {
	switch metric {
	case "", "cosine":
		return "<=>", nil
	case "l2", "euclidean":
		return "<->", nil
	case "inner_product", "ip":
		return "<#>", nil
	default:
		return "", fmt.Errorf("unknown metric %q (use cosine, l2/euclidean, or inner_product/ip)", metric)
	}
}

// prepareQuery validates params and builds the nearest-neighbour SELECT plus
// its args ($1 = query vector). Pure (no DB) so it is fully unit-testable.
func prepareQuery(params map[string]any) (string, []any, error) {
	table, err := kbIdentParam(params, "table", "")
	if err != nil {
		return "", nil, err
	}
	embCol, err := kbIdentParam(params, "embedding_column", kbDefaultEmbeddingColumn)
	if err != nil {
		return "", nil, err
	}

	vector, ok := params["vector"].(string)
	if !ok || vector == "" {
		return "", nil, fmt.Errorf("vector is required (a pgvector literal, e.g. from ai/embed output.vector)")
	}

	metric, _ := params["metric"].(string)
	op, err := kbDistanceOperator(metric)
	if err != nil {
		return "", nil, err
	}

	topK := kbDefaultTopK
	if v, ok := extractInt(params["top_k"]); ok {
		if v <= 0 {
			return "", nil, fmt.Errorf("top_k must be positive, got %d", v)
		}
		topK = v
	}
	if topK > kbMaxTopK {
		topK = kbMaxTopK
	}

	// Columns to return: default to the content column; callers may request more.
	var cols []string
	if raw, ok := params["columns"]; ok {
		list, lErr := kbStringList(raw)
		if lErr != nil {
			return "", nil, fmt.Errorf("columns: %w", lErr)
		}
		for _, c := range list {
			if !kbIdentRe.MatchString(c) {
				return "", nil, fmt.Errorf("column %q is not a valid SQL identifier", c)
			}
		}
		cols = list
	} else {
		contentCol, cErr := kbIdentParam(params, "content_column", kbDefaultContentColumn)
		if cErr != nil {
			return "", nil, cErr
		}
		cols = []string{contentCol}
	}

	selectList := strings.Join(cols, ", ")
	sql := fmt.Sprintf(
		"SELECT %s, (%s %s $1::vector) AS distance FROM %s ORDER BY %s %s $1::vector LIMIT %s",
		selectList, embCol, op, table, embCol, op, strconv.Itoa(topK))
	return sql, []any{vector}, nil
}

func kbStringList(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("[%d] must be a string, got %T", i, item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be an array of strings, got %T", raw)
	}
}

// KBUpsertConnector implements kb/upsert: store document text + embedding
// (and optional JSONB metadata) into a pgvector table, one or many rows.
type KBUpsertConnector struct{}

func (c *KBUpsertConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	connURL, err := extractPostgresURL(params)
	if err != nil {
		return nil, fmt.Errorf("kb/upsert: %w", err)
	}

	query, args, err := prepareUpsert(params)
	if err != nil {
		return nil, fmt.Errorf("kb/upsert: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, kbConnectTimeout)
	conn, err := pgx.Connect(connectCtx, connURL)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("kb/upsert: connecting: %w", err)
	}
	defer conn.Close(ctx)

	out, err := executeExec(ctx, conn, query, args)
	if err != nil {
		return nil, fmt.Errorf("kb/upsert: %w", err)
	}
	return out, nil
}

// KBQueryConnector implements kb/query: nearest-neighbour search over a
// pgvector table for a query embedding, returning the closest rows.
type KBQueryConnector struct{}

func (c *KBQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	connURL, err := extractPostgresURL(params)
	if err != nil {
		return nil, fmt.Errorf("kb/query: %w", err)
	}

	query, args, err := prepareQuery(params)
	if err != nil {
		return nil, fmt.Errorf("kb/query: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, kbConnectTimeout)
	conn, err := pgx.Connect(connectCtx, connURL)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("kb/query: connecting: %w", err)
	}
	defer conn.Close(ctx)

	out, err := executeSelect(ctx, conn, query, args)
	if err != nil {
		return nil, fmt.Errorf("kb/query: %w", err)
	}
	return out, nil
}
