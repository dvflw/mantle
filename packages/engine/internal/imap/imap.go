// Package imap provides shared IMAP connection and search utilities used by
// both the connector package (email/receive, email/delete, email/move,
// email/flag) and the server's email trigger poller.
package imap

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Config holds IMAP connection parameters extracted from a credential.
type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	UseTLS   bool
}

// ParseCredential extracts IMAP config from connector params containing a
// "_credential" key whose value is a map[string]string.
func ParseCredential(params map[string]any) (*Config, error) {
	cred, ok := params["_credential"].(map[string]string)
	if !ok {
		return nil, fmt.Errorf("IMAP credential is required")
	}
	return ParseCredentialMap(cred)
}

// ParseCredentialMap extracts IMAP config from a flat credential map. This is
// used by the email trigger poller which resolves credentials separately.
func ParseCredentialMap(cred map[string]string) (*Config, error) {
	cfg := &Config{
		Host:     cred["host"],
		Port:     cred["port"],
		Username: cred["username"],
		Password: cred["password"],
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("IMAP credential must include 'host'")
	}
	if cfg.Port == "" {
		cfg.Port = "993"
	}
	if useTLS, ok := cred["use_tls"]; ok && useTLS == "false" {
		cfg.UseTLS = false
	} else {
		cfg.UseTLS = true
	}
	return cfg, nil
}

// Dial connects to an IMAP server and authenticates using the provided config.
// It logs a warning when TLS is disabled.
func Dial(cfg *Config) (*imapclient.Client, error) {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	var client *imapclient.Client
	var err error
	if cfg.UseTLS {
		client, err = imapclient.DialTLS(addr, &imapclient.Options{
			TLSConfig: &tls.Config{
				ServerName: cfg.Host,
				MinVersion: tls.VersionTLS12,
			},
		})
	} else {
		slog.Warn("IMAP connection using plaintext (TLS disabled)",
			"host", cfg.Host,
			"port", cfg.Port,
		)
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

// BuildSearchCriteria constructs IMAP search criteria for the given filter
// name. Supported filters: "unseen", "flagged", "recent", "all".
func BuildSearchCriteria(filter string) *imap.SearchCriteria {
	switch filter {
	case "unseen":
		return &imap.SearchCriteria{
			NotFlag: []imap.Flag{imap.FlagSeen},
		}
	case "flagged":
		return &imap.SearchCriteria{
			Flag: []imap.Flag{imap.FlagFlagged},
		}
	case "recent":
		return &imap.SearchCriteria{
			Flag: []imap.Flag{imap.Flag(`\Recent`)},
		}
	default: // "all"
		return &imap.SearchCriteria{}
	}
}
