package connector

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/emersion/go-imap/v2/imapclient"
)

// imapConfig holds IMAP connection parameters extracted from a credential.
type imapConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	UseTLS   bool
}

// parseIMAPCredential extracts IMAP config from a credential map.
func parseIMAPCredential(params map[string]any) (*imapConfig, error) {
	cred, ok := params["_credential"].(map[string]string)
	if !ok {
		return nil, fmt.Errorf("IMAP credential is required")
	}
	cfg := &imapConfig{
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

// imapDial connects to an IMAP server and logs in.
func imapDial(cfg *imapConfig) (*imapclient.Client, error) {
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
