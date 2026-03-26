package connector

import (
	"context"
	"fmt"
	"log"

	"github.com/emersion/go-imap/v2"
)

// EmailDeleteConnector deletes an email message via IMAP.
type EmailDeleteConnector struct{}

// Execute deletes the message identified by uid from the given folder.
//
// Params:
//   - uid         (uint32, required)
//   - folder      (string, default "INBOX")
//   - _credential (required: host, port, username, password, use_tls)
func (c *EmailDeleteConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	cfg, err := parseIMAPCredential(params)
	if err != nil {
		return nil, fmt.Errorf("email/delete: %w", err)
	}

	uid, err := extractUID(params, "uid")
	if err != nil {
		return nil, fmt.Errorf("email/delete: %w", err)
	}

	folder := "INBOX"
	if v, ok := params["folder"].(string); ok && v != "" {
		folder = v
	}

	client, err := imapDial(cfg)
	if err != nil {
		return nil, fmt.Errorf("email/delete: %w", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Select(folder, nil).Wait(); err != nil {
		return nil, fmt.Errorf("email/delete: selecting folder %q: %w", folder, err)
	}

	var uidSet imap.UIDSet
	uidSet.AddNum(imap.UID(uid))

	storeFlags := &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Flags:  []imap.Flag{imap.FlagDeleted},
		Silent: true,
	}
	if err := client.Store(uidSet, storeFlags, nil).Close(); err != nil {
		return nil, fmt.Errorf("email/delete: marking message as deleted: %w", err)
	}

	if err := client.UIDExpunge(uidSet).Close(); err != nil {
		// Fall back to regular EXPUNGE if UIDExpunge is not supported.
		// Warning: EXPUNGE removes ALL messages currently marked \Deleted in the
		// selected mailbox, not just the targeted UID. This may delete additional
		// messages if other messages were marked \Deleted concurrently.
		log.Printf("email/delete: UIDExpunge not supported (uid=%d), falling back to EXPUNGE which removes ALL \\Deleted messages: %v", uid, err)
		if err2 := client.Expunge().Close(); err2 != nil {
			return nil, fmt.Errorf("email/delete: expunging message: %w", err2)
		}
	}

	return map[string]any{
		"deleted": true,
		"uid":     uid,
	}, nil
}
