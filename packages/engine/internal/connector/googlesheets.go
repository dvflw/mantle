package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const googleSheetsBaseURL = "https://sheets.googleapis.com/v4/spreadsheets"

// GoogleSheetsReadRangeConnector reads values from a Google Sheets range.
type GoogleSheetsReadRangeConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *GoogleSheetsReadRangeConnector) getBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return googleSheetsBaseURL
}

func (c *GoogleSheetsReadRangeConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("sheets/read_range: %w", err)
	}

	spreadsheetID, _ := params["spreadsheet_id"].(string)
	if spreadsheetID == "" {
		return nil, fmt.Errorf("sheets/read_range: spreadsheet_id is required")
	}
	rangeStr, _ := params["range"].(string)
	if rangeStr == "" {
		return nil, fmt.Errorf("sheets/read_range: range is required")
	}

	endpoint := fmt.Sprintf("%s/%s/values/%s",
		c.getBaseURL(),
		url.PathEscape(spreadsheetID),
		url.PathEscape(rangeStr),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("sheets/read_range: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("sheets/read_range: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("sheets/read_range: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sheets/read_range: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var result struct {
		Values [][]any `json:"values"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sheets/read_range: parsing response: %w", err)
	}

	return map[string]any{
		"values":    result.Values,
		"row_count": len(result.Values),
	}, nil
}

// GoogleSheetsAppendRowsConnector appends rows to a Google Sheets range.
type GoogleSheetsAppendRowsConnector struct {
	Client  *http.Client
	baseURL string
}

func (c *GoogleSheetsAppendRowsConnector) getBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return googleSheetsBaseURL
}

func (c *GoogleSheetsAppendRowsConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	token, err := extractBearerToken(params)
	if err != nil {
		return nil, fmt.Errorf("sheets/append_rows: %w", err)
	}

	spreadsheetID, _ := params["spreadsheet_id"].(string)
	if spreadsheetID == "" {
		return nil, fmt.Errorf("sheets/append_rows: spreadsheet_id is required")
	}
	rangeStr, _ := params["range"].(string)
	if rangeStr == "" {
		return nil, fmt.Errorf("sheets/append_rows: range is required")
	}
	values, ok := params["values"].([]any)
	if !ok || values == nil {
		return nil, fmt.Errorf("sheets/append_rows: values is required")
	}

	endpoint := fmt.Sprintf("%s/%s/values/%s:append?valueInputOption=USER_ENTERED&insertDataOption=INSERT_ROWS",
		c.getBaseURL(),
		url.PathEscape(spreadsheetID),
		url.PathEscape(rangeStr),
	)

	bodyData, err := json.Marshal(map[string]any{"values": values})
	if err != nil {
		return nil, fmt.Errorf("sheets/append_rows: marshaling body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("sheets/append_rows: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("sheets/append_rows: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("sheets/append_rows: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sheets/append_rows: API returned %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return parseJSONBody(body, "sheets/append_rows")
}
