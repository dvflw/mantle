package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
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

// bqField describes a BigQuery schema field.
type bqField struct {
	Name   string    `json:"name"`
	Type   string    `json:"type"`
	Mode   string    `json:"mode"`
	Fields []bqField `json:"fields"`
}

// bqSchema wraps the fields list returned in a BigQuery query response.
type bqSchema struct {
	Fields []bqField `json:"fields"`
}

// bqRow is a BigQuery row: an ordered list of raw cell values.
type bqRow struct {
	F []json.RawMessage `json:"f"`
}

// bqJobRef identifies a BigQuery job.
type bqJobRef struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
}

// bqQueryResp is the common shape for jobs.query and jobs.getQueryResults responses.
type bqQueryResp struct {
	JobComplete  bool     `json:"jobComplete"`
	JobReference bqJobRef `json:"jobReference"`
	Schema       bqSchema `json:"schema"`
	Rows         []bqRow  `json:"rows"`
	TotalRows    string   `json:"totalRows"`
	PageToken    string   `json:"pageToken"`
}

// BigQueryQueryConnector executes a SQL query on BigQuery.
type BigQueryQueryConnector struct {
	Client       *http.Client
	baseURL      string
	pollInterval time.Duration
}

func (c *BigQueryQueryConnector) getBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return bigQueryBaseURL
}

func (c *BigQueryQueryConnector) getPollInterval() time.Duration {
	if c.pollInterval > 0 {
		return c.pollInterval
	}
	return 500 * time.Millisecond
}

func (c *BigQueryQueryConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	projectID, authToken, err := extractBigQueryCredential(params)
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: %w", err)
	}

	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("bigquery/query: query is required")
	}

	maxRows := 1000
	if m, ok := extractInt(params["max_rows"]); ok && m > 0 {
		maxRows = m
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
		"maxResults":   maxRows,
	})
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: marshaling body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/queries", c.getBaseURL(), projectID)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
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

	var page bqQueryResp
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("bigquery/query: parsing response: %w", err)
	}

	// Poll until the job completes; the caller's context deadline is the overall timeout.
	for !page.JobComplete {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("bigquery/query: %w", ctx.Err())
		case <-time.After(c.getPollInterval()):
		}
		next, err := c.getQueryResults(ctx, authToken, page.JobReference.ProjectID, page.JobReference.JobID, "", maxRows, timeoutMs)
		if err != nil {
			return nil, fmt.Errorf("bigquery/query: polling job: %w", err)
		}
		page = *next
	}

	// Paginate through any remaining pages.
	for page.PageToken != "" && len(page.Rows) < maxRows {
		next, err := c.getQueryResults(ctx, authToken, page.JobReference.ProjectID, page.JobReference.JobID, page.PageToken, maxRows-len(page.Rows), timeoutMs)
		if err != nil {
			return nil, fmt.Errorf("bigquery/query: fetching page: %w", err)
		}
		page.Rows = append(page.Rows, next.Rows...)
		page.PageToken = next.PageToken
	}

	if len(page.Rows) > maxRows {
		page.Rows = page.Rows[:maxRows]
	}

	rows, err := bqParseRows(page.Schema.Fields, page.Rows)
	if err != nil {
		return nil, fmt.Errorf("bigquery/query: parsing rows: %w", err)
	}

	totalRows := len(rows)
	if tr, ok := extractInt(parseIntString(page.TotalRows)); ok {
		totalRows = tr
	}

	return map[string]any{
		"rows":       rows,
		"schema":     page.Schema,
		"total_rows": totalRows,
	}, nil
}

// getQueryResults fetches a page of results for an already-submitted BigQuery job.
func (c *BigQueryQueryConnector) getQueryResults(ctx context.Context, authToken, projectID, jobID, pageToken string, maxResults, timeoutMs int) (*bqQueryResp, error) {
	u := fmt.Sprintf("%s/%s/queries/%s?maxResults=%d&timeoutMs=%d", c.getBaseURL(), projectID, jobID, maxResults, timeoutMs)
	if pageToken != "" {
		u += "&pageToken=" + url.QueryEscape(pageToken)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var page bqQueryResp
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &page, nil
}

// bqParseRows converts BigQuery rows into named maps using schema field metadata.
// NULL values become nil, RECORD types become nested maps, REPEATED fields become slices.
func bqParseRows(fields []bqField, rows []bqRow) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m, err := bqParseRowMap(fields, row.F)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, nil
}

// bqParseRowMap zips schema fields with raw cell values into a named map.
func bqParseRowMap(fields []bqField, cells []json.RawMessage) (map[string]any, error) {
	m := make(map[string]any, len(fields))
	for i, field := range fields {
		if i >= len(cells) {
			m[field.Name] = nil
			continue
		}
		v, err := bqParseValue(field, cells[i])
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}
		m[field.Name] = v
	}
	return m, nil
}

// bqParseValue parses a single BigQuery cell value: {"v": <value>}.
// NULL → nil, RECORD → map, REPEATED → slice, scalars → string.
func bqParseValue(field bqField, raw json.RawMessage) (any, error) {
	var cell struct {
		V json.RawMessage `json:"v"`
	}
	if err := json.Unmarshal(raw, &cell); err != nil {
		return nil, err
	}
	if len(cell.V) == 0 || string(cell.V) == "null" {
		return nil, nil
	}

	if field.Mode == "REPEATED" {
		var items []json.RawMessage
		if err := json.Unmarshal(cell.V, &items); err != nil {
			return nil, fmt.Errorf("parsing repeated field: %w", err)
		}
		elemField := field
		elemField.Mode = "NULLABLE"
		result := make([]any, 0, len(items))
		for _, item := range items {
			v, err := bqParseValue(elemField, item)
			if err != nil {
				return nil, err
			}
			result = append(result, v)
		}
		return result, nil
	}

	if field.Type == "RECORD" || field.Type == "STRUCT" {
		var subRow bqRow
		if err := json.Unmarshal(cell.V, &subRow); err != nil {
			return nil, fmt.Errorf("parsing record field: %w", err)
		}
		return bqParseRowMap(field.Fields, subRow.F)
	}

	// Scalar values come as JSON strings from the BigQuery REST API.
	var s string
	if err := json.Unmarshal(cell.V, &s); err != nil {
		var v any
		if err2 := json.Unmarshal(cell.V, &v); err2 != nil {
			return nil, err
		}
		return v, nil
	}
	return s, nil
}

// parseIntString converts a string integer to int for use with extractInt.
func parseIntString(s string) any {
	if s == "" {
		return nil
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return nil
	}
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
