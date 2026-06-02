package connector

import (
	"encoding/json"
	"fmt"
)

// BrowserSession holds serialized browser state passed between declarative steps.
type BrowserSession struct {
	Cookies      []map[string]any             `json:"cookies"`
	LocalStorage map[string]map[string]string `json:"local_storage"`
	URL          string                       `json:"url"`
}

// extractSession parses the optional session_state param into a *BrowserSession.
// Returns nil (no error) when absent or nil — callers treat nil as "start fresh".
func extractSession(params map[string]any) (*BrowserSession, error) {
	raw, ok := params["session_state"]
	if !ok || raw == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("browser: marshaling session_state: %w", err)
	}
	var s BrowserSession
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("browser: invalid session_state: %w", err)
	}
	return &s, nil
}

// extractTimeoutMs returns the timeout_ms param as an int, defaulting to 30000.
func extractTimeoutMs(params map[string]any) int {
	if v, ok := params["timeout_ms"]; ok {
		switch t := v.(type) {
		case int:
			return t
		case int64:
			return int(t)
		case float64:
			return int(t)
		}
	}
	return 30000
}
