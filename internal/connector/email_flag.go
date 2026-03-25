package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
)

// EmailFlagConnector adds or removes IMAP flags (including custom keywords/tags).
type EmailFlagConnector struct{}

// Execute adds or removes the specified flags on the message identified by uid.
//
// Params:
//   - uid         (uint32, required)
//   - folder      (string, default "INBOX")
//   - flags       ([]string, required — e.g., ["seen", "flagged", "custom-tag"])
//   - action      (string: "add" or "remove", required)
//   - _credential (required: host, port, username, password, use_tls)
func (c *EmailFlagConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	cfg, err := parseIMAPCredential(params)
	if err != nil {
		return nil, fmt.Errorf("email/flag: %w", err)
	}

	uid, err := extractUID(params, "uid")
	if err != nil {
		return nil, fmt.Errorf("email/flag: %w", err)
	}

	folder := "INBOX"
	if v, ok := params["folder"].(string); ok && v != "" {
		folder = v
	}

	flags, err := extractFlagNames(params)
	if err != nil {
		return nil, fmt.Errorf("email/flag: %w", err)
	}

	action, ok := params["action"].(string)
	if !ok || action == "" {
		return nil, fmt.Errorf("email/flag: action is required (\"add\" or \"remove\")")
	}
	action = strings.ToLower(action)
	if action != "add" && action != "remove" {
		return nil, fmt.Errorf("email/flag: action must be \"add\" or \"remove\", got %q", action)
	}

	imapFlags := mapFlagNames(flags)

	client, err := imapDial(cfg)
	if err != nil {
		return nil, fmt.Errorf("email/flag: %w", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Select(folder, nil).Wait(); err != nil {
		return nil, fmt.Errorf("email/flag: selecting folder %q: %w", folder, err)
	}

	var uidSet imap.UIDSet
	uidSet.AddNum(imap.UID(uid))

	op := imap.StoreFlagsAdd
	if action == "remove" {
		op = imap.StoreFlagsDel
	}

	storeFlags := &imap.StoreFlags{
		Op:     op,
		Flags:  imapFlags,
		Silent: true,
	}
	if err := client.Store(uidSet, storeFlags, nil).Close(); err != nil {
		return nil, fmt.Errorf("email/flag: updating flags: %w", err)
	}

	return map[string]any{
		"updated": true,
		"uid":     uid,
		"flags":   flags,
		"action":  action,
	}, nil
}

// extractFlagNames extracts the flags param as a []string from various input types.
func extractFlagNames(params map[string]any) ([]string, error) {
	raw, exists := params["flags"]
	if !exists || raw == nil {
		return nil, fmt.Errorf("flags is required")
	}

	switch v := raw.(type) {
	case []string:
		if len(v) == 0 {
			return nil, fmt.Errorf("flags must not be empty")
		}
		return v, nil
	case []any:
		if len(v) == 0 {
			return nil, fmt.Errorf("flags must not be empty")
		}
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("flags[%d] is not a string", i)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("flags must be a list of strings")
	}
}

// mapFlagNames converts human-readable flag names to IMAP flag constants.
// Known system flags are mapped to their IMAP backslash form; unknown names
// are treated as IMAP keywords and passed through as-is.
func mapFlagNames(names []string) []imap.Flag {
	flags := make([]imap.Flag, 0, len(names))
	for _, name := range names {
		switch strings.ToLower(name) {
		case "seen":
			flags = append(flags, imap.FlagSeen)
		case "flagged":
			flags = append(flags, imap.FlagFlagged)
		case "answered":
			flags = append(flags, imap.FlagAnswered)
		case "deleted":
			flags = append(flags, imap.FlagDeleted)
		case "draft":
			flags = append(flags, imap.FlagDraft)
		default:
			// Custom keyword / tag — pass through as-is.
			flags = append(flags, imap.Flag(name))
		}
	}
	return flags
}
