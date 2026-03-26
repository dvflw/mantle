package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/textproto"
	"sync"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/metrics"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

const (
	defaultEmailPollInterval = 60 * time.Second
	emailReconnectBaseDelay  = 5 * time.Second
	emailReconnectMaxDelay   = 5 * time.Minute
	defaultMaxEmailConns     = 5
)

// EmailTriggerPoller starts one goroutine per email trigger and maintains a
// persistent IMAP connection for each. On new unseen messages it fires the
// associated workflow execution.
type EmailTriggerPoller struct {
	server         *Server
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex
	pollers        map[string]context.CancelFunc // trigger ID → cancel func
	maxConnections int
}

// NewEmailTriggerPoller creates an EmailTriggerPoller attached to the server.
func NewEmailTriggerPoller(s *Server) *EmailTriggerPoller {
	return &EmailTriggerPoller{
		server:         s,
		pollers:        make(map[string]context.CancelFunc),
		maxConnections: defaultMaxEmailConns,
	}
}

// Start loads all email triggers from the DB and starts a goroutine per trigger.
func (e *EmailTriggerPoller) Start(ctx context.Context) error {
	ctx, e.cancel = context.WithCancel(ctx)

	triggers, err := ListEmailTriggers(ctx, e.server.DB)
	if err != nil {
		return fmt.Errorf("email poller: listing triggers: %w", err)
	}

	for _, t := range triggers {
		e.startPoller(ctx, t)
	}

	e.server.Logger.Info("email trigger poller started", "triggers", len(triggers))
	return nil
}

// Reload syncs running pollers against the current set of DB triggers.
// It stops pollers for removed triggers and starts pollers for new ones.
// Called after `mantle apply` updates workflow definitions.
func (e *EmailTriggerPoller) Reload(ctx context.Context) error {
	triggers, err := ListEmailTriggers(ctx, e.server.DB)
	if err != nil {
		return fmt.Errorf("email poller: reload listing triggers: %w", err)
	}

	// Build a set of current trigger IDs.
	current := make(map[string]struct{}, len(triggers))
	for _, t := range triggers {
		current[t.ID] = struct{}{}
	}

	// Stop pollers for triggers that no longer exist.
	e.mu.Lock()
	for id, cancelFn := range e.pollers {
		if _, ok := current[id]; !ok {
			cancelFn()
			delete(e.pollers, id)
			e.server.Logger.Info("email poller: stopped removed trigger", "trigger_id", id)
		}
	}
	e.mu.Unlock()

	// Start pollers for newly added triggers.
	for _, t := range triggers {
		e.startPollerIfNotRunning(ctx, t)
	}

	return nil
}

// Stop cancels all running pollers and waits for them to exit.
func (e *EmailTriggerPoller) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

// startPoller starts a single goroutine for the given trigger, subject to the
// maxConnections limit. Excess triggers are logged and skipped.
// startPollerIfNotRunning acquires the lock, checks if the trigger is already
// running, and starts it if not. The check-and-start is atomic under the lock
// to prevent duplicate pollers from concurrent Reload calls.
func (e *EmailTriggerPoller) startPollerIfNotRunning(ctx context.Context, t TriggerRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, running := e.pollers[t.ID]; running {
		return
	}
	e.startPollerLocked(ctx, t)
}

// startPoller acquires the lock and starts a poller. Used during Start() where
// no existence check is needed (fresh startup).
func (e *EmailTriggerPoller) startPoller(ctx context.Context, t TriggerRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.startPollerLocked(ctx, t)
}

// startPollerLocked starts a single goroutine for the given trigger. Must be
// called with e.mu held. Excess triggers beyond maxConnections are skipped.
func (e *EmailTriggerPoller) startPollerLocked(ctx context.Context, t TriggerRecord) {
	if len(e.pollers) >= e.maxConnections {
		e.server.Logger.Warn("email poller: maxConnections reached, skipping trigger",
			"trigger_id", t.ID,
			"workflow", t.WorkflowName,
			"max", e.maxConnections)
		metrics.EmailTriggersSkippedTotal.Inc()
		return
	}

	pollCtx, cancel := context.WithCancel(ctx) // #nosec G118 -- cancel is stored in e.pollers and called during Stop()/Reload()
	e.pollers[t.ID] = cancel

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer func() {
			e.mu.Lock()
			delete(e.pollers, t.ID)
			e.mu.Unlock()
		}()
		e.runPoller(pollCtx, t)
	}()
}

// runPoller is the main loop for a single email trigger. It establishes an IMAP
// connection with exponential backoff on failure, then polls at the configured
// interval.
func (e *EmailTriggerPoller) runPoller(ctx context.Context, t TriggerRecord) {
	pollInterval := defaultEmailPollInterval
	if t.PollInterval != "" {
		if d, err := time.ParseDuration(t.PollInterval); err == nil && d > 0 {
			pollInterval = d
		}
	}

	folder := t.Folder
	if folder == "" {
		folder = "INBOX"
	}
	filter := t.Filter
	if filter == "" {
		filter = "unseen"
	}

	e.server.Logger.Info("email poller: starting",
		"trigger_id", t.ID,
		"workflow", t.WorkflowName,
		"mailbox", t.Mailbox,
		"folder", folder,
		"poll_interval", pollInterval)

	backoff := emailReconnectBaseDelay
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		client, err := e.dialTrigger(ctx, t)
		if err != nil {
			metrics.EmailConnectionErrorsTotal.WithLabelValues(t.WorkflowName).Inc()
			if e.server.Auditor != nil {
				_ = e.server.Auditor.Emit(ctx, audit.Event{
					Actor:  "email-poller",
					Action: audit.ActionEmailConnectionFailed,
					Resource: audit.Resource{
						Type: "workflow_trigger",
						ID:   t.ID,
					},
					Metadata: map[string]string{
						"workflow": t.WorkflowName,
						"error":    err.Error(),
					},
					TeamID: t.TeamID,
				})
			}
			e.server.Logger.Error("email poller: connection failed, retrying",
				"trigger_id", t.ID,
				"workflow", t.WorkflowName,
				"error", err,
				"retry_in", backoff)

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			// Exponential backoff, capped at max.
			backoff *= 2
			if backoff > emailReconnectMaxDelay {
				backoff = emailReconnectMaxDelay
			}
			continue
		}

		// Connection established — reset backoff.
		backoff = emailReconnectBaseDelay
		metrics.EmailConnectionsActive.Inc()

		if e.server.Auditor != nil {
			_ = e.server.Auditor.Emit(ctx, audit.Event{
				Actor:  "email-poller",
				Action: audit.ActionEmailConnectionEstablished,
				Resource: audit.Resource{
					Type: "workflow_trigger",
					ID:   t.ID,
				},
				Metadata: map[string]string{
					"workflow": t.WorkflowName,
					"mailbox":  t.Mailbox,
				},
				TeamID: t.TeamID,
			})
		}

		// Run the poll loop on the open connection.
		e.pollLoop(ctx, t, client, folder, filter, pollInterval)

		metrics.EmailConnectionsActive.Dec()
		_ = client.Close()

		// If context was cancelled, exit cleanly.
		select {
		case <-ctx.Done():
			return
		default:
			// Connection dropped unexpectedly — reconnect with backoff.
			e.server.Logger.Warn("email poller: connection lost, reconnecting",
				"trigger_id", t.ID,
				"workflow", t.WorkflowName)
		}
	}
}

// pollLoop selects the target folder and polls for new messages until the
// connection fails or the context is cancelled.
func (e *EmailTriggerPoller) pollLoop(
	ctx context.Context,
	t TriggerRecord,
	client *imapclient.Client,
	folder, filter string,
	interval time.Duration,
) {
	if _, err := client.Select(folder, nil).Wait(); err != nil {
		e.server.Logger.Error("email poller: selecting folder failed",
			"trigger_id", t.ID,
			"folder", folder,
			"error", err)
		metrics.EmailPollErrorsTotal.WithLabelValues(t.WorkflowName, "select_folder").Inc()
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Poll immediately, then on the ticker.
	if err := e.poll(ctx, t, client, folder, filter); err != nil {
		// Connection-level error — break out so the outer loop reconnects.
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.poll(ctx, t, client, folder, filter); err != nil {
				// Connection-level error — break out so the outer loop reconnects.
				return
			}
		}
	}
}

// poll performs one IMAP search and fires workflow executions for new messages.
// It returns a non-nil error only for connection-level failures that should
// trigger a reconnect.
func (e *EmailTriggerPoller) poll(ctx context.Context, t TriggerRecord, client *imapclient.Client, folder, filter string) error {
	start := time.Now()

	criteria := buildEmailSearchCriteria(filter)
	searchData, err := client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		metrics.EmailPollErrorsTotal.WithLabelValues(t.WorkflowName, "search").Inc()
		e.server.Logger.Error("email poller: UID search failed",
			"trigger_id", t.ID,
			"workflow", t.WorkflowName,
			"error", err)
		return err
	}

	const maxEmailsPerPoll = 200

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		metrics.EmailPollDuration.WithLabelValues(t.WorkflowName, folder).Observe(time.Since(start).Seconds())
		return nil
	}

	// Cap to avoid memory exhaustion on large mailboxes.
	if len(uids) > maxEmailsPerPoll {
		e.server.Logger.Warn("email poller: truncating UID list",
			"trigger_id", t.ID,
			"total_uids", len(uids),
			"processing", maxEmailsPerPoll)
		uids = uids[:maxEmailsPerPoll]
	}

	// Fetch envelope + body text for each message.
	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(uid)
	}

	bodySection := &imap.FetchItemBodySection{
		Specifier: imap.PartSpecifierText,
		Peek:      true, // do not auto-mark as seen during fetch
	}
	headerSection := &imap.FetchItemBodySection{
		Specifier: imap.PartSpecifierHeader,
		Peek:      true, // do not auto-mark as seen during fetch
	}
	fetchOptions := &imap.FetchOptions{
		Envelope:    true,
		Flags:       true,
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{bodySection, headerSection},
	}

	cmd := client.Fetch(uidSet, fetchOptions)
	var fetchErr error
	for {
		msgData := cmd.Next()
		if msgData == nil {
			break
		}
		buf, err := msgData.Collect()
		if err != nil {
			fetchErr = err
			break
		}

		inputs := buildEmailTriggerInputs(buf, folder)
		e.fireWorkflow(ctx, t, inputs, imap.UID(buf.UID))
	}
	if closeErr := cmd.Close(); closeErr != nil && fetchErr == nil {
		fetchErr = closeErr
	}

	if fetchErr != nil {
		metrics.EmailPollErrorsTotal.WithLabelValues(t.WorkflowName, "fetch").Inc()
		e.server.Logger.Error("email poller: fetch failed",
			"trigger_id", t.ID,
			"workflow", t.WorkflowName,
			"error", fetchErr)
		return fetchErr
	}

	// Mark fetched messages as seen so they are not re-triggered on the next poll.
	storeFlags := &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Flags:  []imap.Flag{imap.FlagSeen},
		Silent: true,
	}
	if err := client.Store(uidSet, storeFlags, nil).Close(); err != nil {
		// Non-fatal: log the error but do not reconnect. The messages may fire
		// again on the next poll, which is safer than losing them.
		e.server.Logger.Warn("email poller: marking messages as seen failed",
			"trigger_id", t.ID,
			"workflow", t.WorkflowName,
			"uids", fmt.Sprintf("%v", uids),
			"error", err)
		metrics.EmailPollErrorsTotal.WithLabelValues(t.WorkflowName, "mark_seen").Inc()
	}

	metrics.EmailPollDuration.WithLabelValues(t.WorkflowName, folder).Observe(time.Since(start).Seconds())
	return nil
}

// fireWorkflow launches a single workflow execution for one email message.
func (e *EmailTriggerPoller) fireWorkflow(ctx context.Context, t TriggerRecord, inputs map[string]any, uid imap.UID) {
	teamCtx := auth.WithUser(ctx, &auth.User{TeamID: t.TeamID})

	execID, err := e.server.executeWorkflow(teamCtx, t.WorkflowName, t.WorkflowVersion, inputs)
	if err != nil {
		e.server.Logger.Error("email poller: execution failed",
			"trigger_id", t.ID,
			"workflow", t.WorkflowName,
			"uid", uid,
			"error", err)
		metrics.EmailPollErrorsTotal.WithLabelValues(t.WorkflowName, "execute").Inc()
		return
	}

	metrics.EmailTriggersTotal.WithLabelValues(t.WorkflowName).Inc()

	e.server.Logger.Info("email poller: fired workflow",
		"trigger_id", t.ID,
		"workflow", t.WorkflowName,
		"execution_id", execID,
		"uid", uid)

	if e.server.Auditor != nil {
		_ = e.server.Auditor.Emit(teamCtx, audit.Event{
			Actor:  "email-poller",
			Action: audit.ActionEmailTriggerFired,
			Resource: audit.Resource{
				Type: "workflow_execution",
				ID:   execID,
			},
			Metadata: map[string]string{
				"workflow":     t.WorkflowName,
				"trigger_id":   t.ID,
				"message_uid":  fmt.Sprintf("%d", uid),
			},
			TeamID: t.TeamID,
		})
	}
}

// dialTrigger resolves the mailbox credential and establishes an IMAP connection.
func (e *EmailTriggerPoller) dialTrigger(ctx context.Context, t TriggerRecord) (*imapclient.Client, error) {
	if t.Mailbox == "" {
		return nil, fmt.Errorf("email trigger %q has no mailbox credential configured", t.ID)
	}

	if e.server.Engine == nil || e.server.Engine.Resolver == nil {
		return nil, fmt.Errorf("secret resolver not available")
	}

	cred, err := e.server.Engine.Resolver.Resolve(ctx, t.Mailbox)
	if err != nil {
		return nil, fmt.Errorf("resolving mailbox credential %q: %w", t.Mailbox, err)
	}

	cfg := &imapDialConfig{
		Host:     cred["host"],
		Port:     cred["port"],
		Username: cred["username"],
		Password: cred["password"],
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("mailbox credential %q missing 'host' field", t.Mailbox)
	}
	if cfg.Port == "" {
		cfg.Port = "993"
	}
	cfg.UseTLS = cred["use_tls"] != "false"

	return dialIMAP(cfg)
}

// imapDialConfig holds parameters for dialling an IMAP server.
// It mirrors connector.imapConfig but is defined here to avoid a cross-package
// dependency on an unexported type.
type imapDialConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	UseTLS   bool
}

// dialIMAP connects to an IMAP server and authenticates.
func dialIMAP(cfg *imapDialConfig) (*imapclient.Client, error) {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	var (
		client *imapclient.Client
		err    error
	)
	if cfg.UseTLS {
		client, err = imapclient.DialTLS(addr, &imapclient.Options{
			TLSConfig: &tls.Config{
				ServerName: cfg.Host,
				MinVersion: tls.VersionTLS12,
			},
		})
	} else {
		client, err = imapclient.DialInsecure(addr, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to IMAP server %s: %w", addr, err)
	}
	if err := client.Login(cfg.Username, cfg.Password).Wait(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("IMAP login failed: %w", err)
	}
	return client, nil
}

// buildEmailSearchCriteria constructs IMAP search criteria matching the
// connector package's filter semantics.
func buildEmailSearchCriteria(filter string) *imap.SearchCriteria {
	switch filter {
	case "flagged":
		return &imap.SearchCriteria{Flag: []imap.Flag{imap.FlagFlagged}}
	case "recent":
		return &imap.SearchCriteria{Flag: []imap.Flag{imap.Flag(`\Recent`)}}
	case "all":
		return &imap.SearchCriteria{}
	default: // "unseen"
		return &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}
	}
}

// buildEmailTriggerInputs converts a fetched IMAP message buffer into the
// workflow input map passed as trigger context.
func buildEmailTriggerInputs(buf *imapclient.FetchMessageBuffer, folder string) map[string]any {
	trigger := map[string]any{
		"type":       "email",
		"folder":     folder,
		"uid":        uint32(buf.UID),
		"from":       "",
		"to":         []string{},
		"cc":         []string{},
		"subject":    "",
		"body":       "",
		"headers":    map[string]string{},
		"message_id": "",
		"date":       "",
	}

	if env := buf.Envelope; env != nil {
		trigger["message_id"] = env.MessageID
		trigger["subject"] = env.Subject
		if !env.Date.IsZero() {
			trigger["date"] = env.Date.UTC().Format(time.RFC3339)
		}
		if len(env.From) > 0 {
			trigger["from"] = formatTriggerAddress(&env.From[0])
		}
		trigger["to"] = triggerAddressList(env.To)
		trigger["cc"] = triggerAddressList(env.Cc)
	}

	// Extract body text from the TEXT body section.
	// Do NOT fall back to buf.BodySection[0] — that could be the HEADER section.
	bodySection := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierText}
	if rawText := buf.FindBodySection(bodySection); rawText != nil {
		trigger["body"] = extractEmailBody(rawText)
	}

	// Extract headers from the HEADER body section.
	headerSection := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader}
	if rawHeaders := buf.FindBodySection(headerSection); rawHeaders != nil {
		trigger["headers"] = parseRawHeaders(rawHeaders)
	}

	return map[string]any{"trigger": trigger}
}

// formatTriggerAddress formats an imap.Address as "Name <user@host>" or "user@host".
func formatTriggerAddress(addr *imap.Address) string {
	email := addr.Addr()
	if addr.Name != "" {
		return addr.Name + " <" + email + ">"
	}
	return email
}

// triggerAddressList converts a slice of imap.Address to formatted strings.
func triggerAddressList(addrs []imap.Address) []string {
	out := make([]string, 0, len(addrs))
	for i := range addrs {
		if s := formatTriggerAddress(&addrs[i]); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseRawHeaders parses RFC 2822 raw header bytes into a flat map keyed by
// canonical MIME header name (title-case, e.g. "Content-Type"). Folded header
// lines are unfolded by the textproto reader automatically. Only the first
// value is kept for headers that appear multiple times.
func parseRawHeaders(raw []byte) map[string]string {
	headers := make(map[string]string)
	if len(raw) == 0 {
		return headers
	}
	tp := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw)))
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil && len(mimeHeader) == 0 {
		return headers
	}
	for key, values := range mimeHeader {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

// extractEmailBody trims trailing whitespace from a raw body byte slice.
func extractEmailBody(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	s := string(raw)
	// Trim trailing CRLF/LF only.
	for len(s) > 0 && (s[len(s)-1] == '\r' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

