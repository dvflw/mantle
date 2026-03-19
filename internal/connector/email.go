package connector

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// EmailSendConnector sends emails via SMTP.
type EmailSendConnector struct{}

func (c *EmailSendConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	// Extract SMTP credentials.
	var username, password, host, port string
	if cred, ok := params["_credential"].(map[string]string); ok {
		username = cred["username"]
		password = cred["password"]
		host = cred["host"]
		port = cred["port"]
		delete(params, "_credential")
	}

	// Fallback to params for host/port if not in credential.
	if host == "" {
		host, _ = params["smtp_host"].(string)
	}
	if port == "" {
		port, _ = params["smtp_port"].(string)
	}

	if host == "" {
		return nil, fmt.Errorf("email/send: smtp host is required (provide via credential or smtp_host param)")
	}
	if port == "" {
		port = "587"
	}

	// Extract required params.
	from, _ := params["from"].(string)
	if from == "" {
		return nil, fmt.Errorf("email/send: from is required")
	}

	subject, _ := params["subject"].(string)
	if subject == "" {
		return nil, fmt.Errorf("email/send: subject is required")
	}

	body, _ := params["body"].(string)
	if body == "" {
		return nil, fmt.Errorf("email/send: body is required")
	}

	recipients, err := parseRecipients(params["to"])
	if err != nil {
		return nil, fmt.Errorf("email/send: %w", err)
	}

	isHTML, _ := params["html"].(bool)

	msg := buildMessage(from, recipients, subject, body, isHTML)

	addr := net.JoinHostPort(host, port)

	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}

	if err := smtp.SendMail(addr, auth, from, recipients, msg); err != nil {
		return nil, fmt.Errorf("email/send: %w", err)
	}

	return map[string]any{
		"sent":    true,
		"to":      strings.Join(recipients, ", "),
		"subject": subject,
	}, nil
}

// parseRecipients accepts a string or []any (from YAML/JSON unmarshalling) and
// returns a validated slice of recipient addresses.
func parseRecipients(v any) ([]string, error) {
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil, fmt.Errorf("to is required")
		}
		return []string{val}, nil
	case []any:
		if len(val) == 0 {
			return nil, fmt.Errorf("to is required")
		}
		out := make([]string, 0, len(val))
		for i, item := range val {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, fmt.Errorf("to[%d] must be a non-empty string", i)
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		if len(val) == 0 {
			return nil, fmt.Errorf("to is required")
		}
		return val, nil
	case nil:
		return nil, fmt.Errorf("to is required")
	default:
		return nil, fmt.Errorf("to must be a string or list of strings, got %T", v)
	}
}

// buildMessage constructs an RFC 2822 compliant email message with MIME headers.
func buildMessage(from string, to []string, subject, body string, isHTML bool) []byte {
	var b strings.Builder

	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")

	b.WriteString("To: ")
	b.WriteString(strings.Join(to, ", "))
	b.WriteString("\r\n")

	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\r\n")

	b.WriteString("MIME-Version: 1.0\r\n")

	if isHTML {
		b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	} else {
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	}

	b.WriteString("\r\n")
	b.WriteString(body)

	return []byte(b.String())
}
