package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const bigQueryBaseURL = "https://bigquery.googleapis.com/bigquery/v2/projects"

// extractBigQueryCredential extracts BigQuery credentials from params.
// Credential shape: {project_id, token}
func extractBigQueryCredential(params map[string]any) (projectID, token string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", fmt.Errorf("credential is required")
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
		return "", "", fmt.Errorf("credential is required")
	}

	projectID = cred["project_id"]
	if projectID == "" {
		return "", "", fmt.Errorf("credential must contain a 'project_id' field")
	}
	token = cred["token"]
	if token == "" {
		return "", "", fmt.Errorf("credential must contain a 'token' field")
	}
	return projectID, token, nil
}

// BigQueryQueryConnector executes a SQL query on BigQuery.
type BigQueryQueryConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *BigQueryQueryConnector) getBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return bigQueryBaseURL
}

func (c *BigQueryQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	projectID, token, err := extractBigQueryCredential(params)
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: %w", err)
	}

	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("bigquery/query: query is required")
	}

	timeoutMs := 30000
	if tm, ok := extractInt(params["timeout_ms"]); ok && tm > 0 {
		timeoutMs = tm
	}

	useLegacySQL := false
	if ul, ok := params["use_legacy_sql"].(bool); ok {
		useLegacySQL = ul
	}

	payload, err := json.Marshal(map[string]any{
		"query":        query,
		"timeoutMs":    timeoutMs,
		"useLegacySql": useLegacySQL,
	})
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: marshaling body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/queries", c.getBaseURL(), projectID)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bigquery/query: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	// Parse the raw response and flatten BigQuery row format.
	var raw struct {
		Schema    map[string]any   `json:"schema"`
		Rows      []map[string]any `json:"rows"`
		TotalRows string           `json:"totalRows"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("bigquery/query: parsing response: %w", err)
	}

	// Flatten rows: each row is {"f": [{"v": "val"}, ...]}
	rows := flattenBigQueryRows(raw.Rows)

	totalRows := len(rows)
	if tr, ok := extractInt(parseIntString(raw.TotalRows)); ok {
		totalRows = tr
	}

	return map[string]any{
		"rows":       rows,
		"schema":     raw.Schema,
		"total_rows": totalRows,
	}, nil
}

// flattenBigQueryRows converts BQ's nested row format into [][]string.
func flattenBigQueryRows(rows []map[string]any) [][]string {
	result := make([][]string, 0, len(rows))
	for _, row := range rows {
		fields, _ := row["f"].([]any)
		rowVals := make([]string, 0, len(fields))
		for _, f := range fields {
			fm, _ := f.(map[string]any)
			v, _ := fm["v"].(string)
			rowVals = append(rowVals, v)
		}
		result = append(result, rowVals)
	}
	return result
}

// parseIntString converts a string integer to int for use with extractInt.
func parseIntString(s string) any {
	if s == "" {
		return nil
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// BigQueryInsertRowsConnector inserts rows into a BigQuery table via streaming insertAll.
type BigQueryInsertRowsConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *BigQueryInsertRowsConnector) getBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return bigQueryBaseURL
}

func (c *BigQueryInsertRowsConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	projectID, token, err := extractBigQueryCredential(params)
	if err != nil {
		return nil, fmt.Errorf("bigquery/insert_rows: %w", err)
	}

	datasetID, _ := params["dataset_id"].(string)
	if datasetID == "" {
		return nil, fmt.Errorf("bigquery/insert_rows: dataset_id is required")
	}
	tableID, _ := params["table_id"].(string)
	if tableID == "" {
		return nil, fmt.Errorf("bigquery/insert_rows: table_id is required")
	}
	rows, ok := params["rows"].([]any)
	if !ok || rows == nil {
		return nil, fmt.Errorf("bigquery/insert_rows: rows is required")
	}

	// Wrap each row in {"json": row}.
	insertRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		insertRows = append(insertRows, map[string]any{"json": row})
	}

	payload, err := json.Marshal(map[string]any{
		"rows": insertRows,
	})
	if err != nil {
		return nil, fmt.Errorf("bigquery/insert_rows: marshaling body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/datasets/%s/tables/%s/insertAll",
		c.getBaseURL(), projectID, datasetID, tableID)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("bigquery/insert_rows: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("bigquery/insert_rows: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("bigquery/insert_rows: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bigquery/insert_rows: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "bigquery/insert_rows")
}
