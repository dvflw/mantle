package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const twilioBaseURL = "https://api.twilio.com/2010-04-01"

// TwilioSMSConnector sends an SMS message via Twilio.
type TwilioSMSConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *TwilioSMSConnector) apiURL(accountSID, path string) string {
	base := c.baseURL
	if base == "" {
		base = twilioBaseURL
	}
	return fmt.Sprintf("%s/Accounts/%s%s", base, url.PathEscape(accountSID), path)
}

func (c *TwilioSMSConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	accountSID, authToken, err := extractTwilioCredential(params)
	if err != nil {
		return nil, fmt.Errorf("twilio/sms: %w", err)
	}

	to, _ := params["to"].(string)
	if to == "" {
		return nil, fmt.Errorf("twilio/sms: to is required")
	}
	from, _ := params["from"].(string)
	if from == "" {
		return nil, fmt.Errorf("twilio/sms: from is required")
	}
	body, _ := params["body"].(string)
	if body == "" {
		return nil, fmt.Errorf("twilio/sms: body is required")
	}

	form := url.Values{}
	form.Set("To", to)
	form.Set("From", from)
	form.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(accountSID, "/Messages.json"), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twilio/sms: creating request: %w", err)
	}
	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio/sms: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("twilio/sms: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("twilio/sms: Twilio API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var msg struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
		To     string `json:"to"`
		From   string `json:"from"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(respBody, &msg); err != nil {
		return nil, fmt.Errorf("twilio/sms: parsing response: %w", err)
	}

	return map[string]any{
		"sid":    msg.SID,
		"status": msg.Status,
		"to":     msg.To,
		"from":   msg.From,
		"body":   msg.Body,
	}, nil
}

// TwilioCallConnector initiates an outbound phone call via Twilio.
type TwilioCallConnector struct {
	Client  *http.Client
	baseURL string // override for testing
}

func (c *TwilioCallConnector) apiURL(accountSID, path string) string {
	base := c.baseURL
	if base == "" {
		base = twilioBaseURL
	}
	return fmt.Sprintf("%s/Accounts/%s%s", base, url.PathEscape(accountSID), path)
}

func (c *TwilioCallConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	accountSID, authToken, err := extractTwilioCredential(params)
	if err != nil {
		return nil, fmt.Errorf("twilio/call: %w", err)
	}

	to, _ := params["to"].(string)
	if to == "" {
		return nil, fmt.Errorf("twilio/call: to is required")
	}
	from, _ := params["from"].(string)
	if from == "" {
		return nil, fmt.Errorf("twilio/call: from is required")
	}

	callURL, _ := params["url"].(string)
	twiml, _ := params["twiml"].(string)
	if callURL == "" && twiml == "" {
		return nil, fmt.Errorf("twilio/call: url or twiml is required")
	}
	if callURL != "" && twiml != "" {
		return nil, fmt.Errorf("twilio/call: url and twiml are mutually exclusive; provide only one")
	}

	form := url.Values{}
	form.Set("To", to)
	form.Set("From", from)
	if callURL != "" {
		form.Set("Url", callURL)
	} else {
		form.Set("Twiml", twiml)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL(accountSID, "/Calls.json"), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twilio/call: creating request: %w", err)
	}
	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient(c.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio/call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, DefaultMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("twilio/call: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("twilio/call: Twilio API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var call struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
		To     string `json:"to"`
		From   string `json:"from"`
	}
	if err := json.Unmarshal(respBody, &call); err != nil {
		return nil, fmt.Errorf("twilio/call: parsing response: %w", err)
	}

	return map[string]any{
		"sid":    call.SID,
		"status": call.Status,
		"to":     call.To,
		"from":   call.From,
	}, nil
}

// extractTwilioCredential extracts account_sid and auth_token from _credential.
func extractTwilioCredential(params map[string]any) (accountSID, authToken string, err error) {
	raw, ok := params["_credential"]
	if !ok || raw == nil {
		return "", "", fmt.Errorf("credential is required")
	}
	delete(params, "_credential")

	switch cred := raw.(type) {
	case map[string]string:
		accountSID = cred["account_sid"]
		authToken = cred["auth_token"]
	case map[string]any:
		accountSID, _ = cred["account_sid"].(string)
		authToken, _ = cred["auth_token"].(string)
	default:
		return "", "", fmt.Errorf("credential is required")
	}
	if accountSID == "" {
		return "", "", fmt.Errorf("credential must contain an 'account_sid' field")
	}
	if authToken == "" {
		return "", "", fmt.Errorf("credential must contain an 'auth_token' field")
	}
	return accountSID, authToken, nil
}
