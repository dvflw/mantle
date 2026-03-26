package connector

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	imaplib "github.com/dvflw/mantle/internal/imap"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// EmailReceiveConnector fetches messages from an IMAP mailbox.
type EmailReceiveConnector struct{}

// Execute fetches email messages from the specified IMAP folder.
//
// Params:
//   - folder      (string, default "INBOX")
//   - filter      (string: "unseen"|"all"|"flagged"|"recent", default "unseen")
//   - limit       (int, default 10)
//   - mark_seen   (bool, default false)
//   - _credential (required: host, port, username, password, use_tls)
func (c *EmailReceiveConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	cfg, err := parseIMAPCredential(params)
	if err != nil {
		return nil, fmt.Errorf("email/receive: %w", err)
	}

	folder := "INBOX"
	if v, ok := params["folder"].(string); ok && v != "" {
		folder = v
	}

	filter := "unseen"
	if v, ok := params["filter"].(string); ok && v != "" {
		filter = v
	}

	const maxEmailFetchLimit = 200

	limit := 10
	switch v := params["limit"].(type) {
	case int:
		if v > 0 {
			limit = v
		}
	case float64:
		if v > 0 {
			limit = int(v)
		}
	}
	if limit > maxEmailFetchLimit {
		limit = maxEmailFetchLimit
	}

	markSeen := false
	if v, ok := params["mark_seen"].(bool); ok {
		markSeen = v
	}

	client, err := imapDial(cfg)
	if err != nil {
		return nil, fmt.Errorf("email/receive: %w", err)
	}
	defer func() { _ = client.Close() }()

	// Select the folder.
	if _, err := client.Select(folder, nil).Wait(); err != nil {
		return nil, fmt.Errorf("email/receive: selecting folder %q: %w", folder, err)
	}

	// Build search criteria based on the filter.
	criteria := imaplib.BuildSearchCriteria(filter)

	// Use UID search so we can build a UIDSet for Fetch.
	searchData, err := client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("email/receive: search failed: %w", err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return map[string]any{
			"message_count": 0,
			"messages":      []map[string]any{},
		}, nil
	}

	// Apply limit — take the last N (most recent) UIDs.
	if len(uids) > limit {
		uids = uids[len(uids)-limit:]
	}

	// Build a UIDSet for the fetch.
	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(uid)
	}

	// Fetch envelope, flags, UID, and the full body text.
	bodySection := &imap.FetchItemBodySection{
		Specifier: imap.PartSpecifierText,
		Peek:      !markSeen,
	}
	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		Flags:    true,
		UID:      true,
		BodySection: []*imap.FetchItemBodySection{
			bodySection,
		},
	}

	messages, err := fetchMessages(client, uidSet, fetchOptions, !markSeen)
	if err != nil {
		return nil, fmt.Errorf("email/receive: fetch failed: %w", err)
	}

	// Optionally mark all fetched messages as \Seen.
	// (When Peek is false the server does this automatically during FETCH, but
	// we mark explicitly here too in case the server did not do so.)
	if markSeen && len(uids) > 0 {
		storeFlags := &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Flags:  []imap.Flag{imap.FlagSeen},
			Silent: true,
		}
		if err := client.Store(uidSet, storeFlags, nil).Close(); err != nil {
			return nil, fmt.Errorf("email/receive: marking messages as seen: %w", err)
		}
	}

	return map[string]any{
		"message_count": len(messages),
		"messages":      messages,
	}, nil
}

// fetchMessages executes the FETCH command and converts the results into the
// output map format expected by the connector.
func fetchMessages(client *imapclient.Client, uidSet imap.UIDSet, opts *imap.FetchOptions, peek bool) ([]map[string]any, error) {
	cmd := client.Fetch(uidSet, opts)

	var out []map[string]any

	for {
		msgData := cmd.Next()
		if msgData == nil {
			break
		}

		buf, err := msgData.Collect()
		if err != nil {
			return nil, err
		}

		msg := messageBufferToMap(buf, peek)
		out = append(out, msg)
	}

	if err := cmd.Close(); err != nil {
		return nil, err
	}

	return out, nil
}

// messageBufferToMap converts a FetchMessageBuffer into the output map format.
// peek must match the Peek value used in the FetchItemBodySection so that
// FindBodySection can locate the correct section by exact struct comparison.
func messageBufferToMap(buf *imapclient.FetchMessageBuffer, peek bool) map[string]any {
	msg := map[string]any{
		"message_id": "",
		"from":       "",
		"to":         []string{},
		"cc":         []string{},
		"subject":    "",
		"body":       "",
		"date":       "",
		"headers":    map[string]string{},
		"flags":      []string{},
		"uid":        uint32(buf.UID),
	}

	// Populate envelope fields.
	if env := buf.Envelope; env != nil {
		msg["message_id"] = env.MessageID
		msg["subject"] = env.Subject
		if !env.Date.IsZero() {
			msg["date"] = env.Date.UTC().Format(time.RFC3339)
		}
		if len(env.From) > 0 {
			msg["from"] = formatAddress(&env.From[0])
		}
		msg["to"] = addressList(env.To)
		msg["cc"] = addressList(env.Cc)
	}

	// Populate flags.
	if len(buf.Flags) > 0 {
		flags := make([]string, len(buf.Flags))
		for i, f := range buf.Flags {
			flags[i] = string(f)
		}
		msg["flags"] = flags
	}

	// Populate body text from the TEXT body section, and extract headers from
	// the raw bytes if a full body section is present.
	bodySection := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierText, Peek: peek}
	rawText := buf.FindBodySection(bodySection)
	if rawText != nil {
		msg["body"] = extractBodyText(rawText)
	} else if len(buf.BodySection) > 0 {
		// Fallback: use the first available body section.
		msg["body"] = extractBodyText(buf.BodySection[0].Bytes)
	}

	return msg
}

// formatAddress formats an imap.Address as "Name <user@host>" or "user@host".
func formatAddress(addr *imap.Address) string {
	email := addr.Addr()
	if addr.Name != "" {
		return addr.Name + " <" + email + ">"
	}
	return email
}

// addressList converts a slice of imap.Address to a slice of formatted strings.
func addressList(addrs []imap.Address) []string {
	if len(addrs) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(addrs))
	for i := range addrs {
		if s := formatAddress(&addrs[i]); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// extractBodyText attempts to parse an RFC 2822 message body from raw bytes.
// It returns the body text, or the raw bytes as a string if parsing fails.
func extractBodyText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	// The TEXT section already excludes the header, so the raw bytes are the
	// body directly. However, some servers return the complete message; try
	// parsing it as a full message to strip headers if present.
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// Not a full message — treat the bytes as body text directly.
		return strings.TrimRight(string(raw), "\r\n")
	}
	body, err := io.ReadAll(m.Body)
	if err != nil {
		return strings.TrimRight(string(raw), "\r\n")
	}
	return strings.TrimRight(string(body), "\r\n")
}
