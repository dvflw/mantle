package connector

import (
	imaplib "github.com/dvflw/mantle/internal/imap"
	"github.com/emersion/go-imap/v2/imapclient"
)

// parseIMAPCredential extracts IMAP config from connector params.
// It delegates to the shared imap package.
func parseIMAPCredential(params map[string]any) (*imaplib.Config, error) {
	return imaplib.ParseCredential(params)
}

// imapDial connects to an IMAP server and logs in.
// It delegates to the shared imap package.
func imapDial(cfg *imaplib.Config) (*imapclient.Client, error) {
	return imaplib.Dial(cfg)
}
