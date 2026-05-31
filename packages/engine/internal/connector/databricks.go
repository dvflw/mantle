package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// extractDatabricksCredential extracts Databricks credentials from params.
// Credential shape: {host, token}
func extractDatabricksCredential(params map[string]any) (host, token string, err error) {
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

	host = cred["host"]
	if host == "" {
		return "", "", fmt.Errorf("credential must contain a 'host' field")
	}
	token = cred["token"]
	if token == "" {
		return "", "", fmt.Errorf("credential must contain a 'token' field")
	}
	return host, token, nil
}

// DatabricksExecuteSQLConnector executes a SQL statement on a Databricks SQL warehouse.
type DatabricksExecuteSQLConnector struct {
	Client *http.Client
}

func (c *DatabricksExecuteSQLConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	host, token, err := extractDatabricksCredential(params)
	if err != nil {
		return nil, fmt.Errorf("databricks/execute_sql: %w", err)
	}

	warehouseID, _ := params["warehouse_id"].(string)
	if warehouseID == "" {
		return nil, fmt.Errorf("databricks/execute_sql: warehouse_id is required")
	}
	statement, _ := params["statement"].(string)
	if statement == "" {
		return nil, fmt.Errorf("databricks/execute_sql: statement is required")
	}

	timeoutSeconds := 30
	if ts, ok := extractInt(params["timeout_seconds"]); ok && ts > 0 {
		timeoutSeconds = ts
	}

	bodyMap := map[string]any{
		"warehouse_id":   warehouseID,
		"statement":      statement,
		"wait_timeout":   fmt.Sprintf("%ds", timeoutSeconds),
	}
	if catalog, ok := params["catalog"].(string); ok && catalog != "" {
		bodyMap["catalog"] = catalog
	}
	if schema, ok := params["schema"].(string); ok && schema != "" {
		bodyMap["schema"] = schema
	}

	payload, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("databricks/execute_sql: marshaling body: %w", err)
	}

	endpoint := host + "/api/2.0/sql/statements"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("databricks/execute_sql: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("databricks/execute_sql: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("databricks/execute_sql: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("databricks/execute_sql: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "databricks/execute_sql")
}

// DatabricksSubmitJobConnector submits a one-time job run on Databricks.
type DatabricksSubmitJobConnector struct {
	Client *http.Client
}

func (c *DatabricksSubmitJobConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	host, token, err := extractDatabricksCredential(params)
	if err != nil {
		return nil, fmt.Errorf("databricks/submit_job: %w", err)
	}

	tasks, ok := params["tasks"].([]any)
	if !ok || tasks == nil {
		return nil, fmt.Errorf("databricks/submit_job: tasks is required")
	}

	bodyMap := map[string]any{
		"tasks": tasks,
	}
	if runName, ok := params["run_name"].(string); ok && runName != "" {
		bodyMap["run_name"] = runName
	}
	if ts, ok := extractInt(params["timeout_seconds"]); ok && ts > 0 {
		bodyMap["timeout_seconds"] = ts
	}

	payload, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("databricks/submit_job: marshaling body: %w", err)
	}

	endpoint := host + "/api/2.1/jobs/runs/submit"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("databricks/submit_job: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("databricks/submit_job: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("databricks/submit_job: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("databricks/submit_job: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "databricks/submit_job")
}
