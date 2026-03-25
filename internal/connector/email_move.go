package connector

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2"
)

// EmailMoveConnector moves an email message from one IMAP folder to another.
type EmailMoveConnector struct{}

// Execute moves the message identified by uid from source_folder to target_folder.
//
// Params:
//   - uid           (uint32, required)
//   - source_folder (string, default "INBOX")
//   - target_folder (string, required)
//   - _credential   (required: host, port, username, password, use_tls)
func (c *EmailMoveConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	cfg, err := parseIMAPCredential(params)
	if err != nil {
		return nil, fmt.Errorf("email/move: %w", err)
	}

	uid, err := extractUID(params, "uid")
	if err != nil {
		return nil, fmt.Errorf("email/move: %w", err)
	}

	sourceFolder := "INBOX"
	if v, ok := params["source_folder"].(string); ok && v != "" {
		sourceFolder = v
	}

	targetFolder := ""
	if v, ok := params["target_folder"].(string); ok && v != "" {
		targetFolder = v
	}
	if targetFolder == "" {
		return nil, fmt.Errorf("email/move: target_folder is required")
	}

	client, err := imapDial(cfg)
	if err != nil {
		return nil, fmt.Errorf("email/move: %w", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Select(sourceFolder, nil).Wait(); err != nil {
		return nil, fmt.Errorf("email/move: selecting folder %q: %w", sourceFolder, err)
	}

	var uidSet imap.UIDSet
	uidSet.AddNum(imap.UID(uid))

	if _, err := client.Move(uidSet, targetFolder).Wait(); err != nil {
		return nil, fmt.Errorf("email/move: moving message: %w", err)
	}

	return map[string]any{
		"moved":         true,
		"uid":           uid,
		"target_folder": targetFolder,
	}, nil
}

// extractUID extracts a uint32 UID value from params, accepting uint32 or float64.
func extractUID(params map[string]any, key string) (uint32, error) {
	switch v := params[key].(type) {
	case uint32:
		if v == 0 {
			return 0, fmt.Errorf("%s must be a non-zero UID", key)
		}
		return v, nil
	case float64:
		if v <= 0 {
			return 0, fmt.Errorf("%s must be a non-zero UID", key)
		}
		return uint32(v), nil
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("%s must be a non-zero UID", key)
		}
		return uint32(v), nil
	case int64:
		if v <= 0 {
			return 0, fmt.Errorf("%s must be a non-zero UID", key)
		}
		return uint32(v), nil
	default:
		return 0, fmt.Errorf("%s is required and must be a uint32", key)
	}
}
