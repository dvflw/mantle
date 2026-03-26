package connector

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	imaplib "github.com/dvflw/mantle/internal/imap"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupGreenMail starts a GreenMail IMAP container and returns the host and
// plain-text IMAP port (3143). GreenMail auto-creates accounts on first login.
// The container is terminated when the test finishes.
func setupGreenMail(t *testing.T) (host string, port string) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "greenmail/standalone:2.0.1",
		ExposedPorts: []string{"3143/tcp"},
		Env: map[string]string{
			// Allow any user to log in automatically (GreenMail default behaviour).
			"GREENMAIL_OPTS": "-Dgreenmail.setup.test.all -Dgreenmail.hostname=0.0.0.0 -Dgreenmail.auth.disabled=true",
		},
		WaitingFor: wait.ForLog("Starting GreenMail").
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("Could not start GreenMail container (CI): %v", err)
		}
		t.Skipf("Could not start GreenMail container (skipping integration test): %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate GreenMail container: %v", err)
		}
	})

	mappedHost, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "3143")
	if err != nil {
		t.Fatalf("Failed to get mapped IMAP port: %v", err)
	}

	return mappedHost, mappedPort.Port()
}

func TestIMAPDial(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host, port := setupGreenMail(t)

	cfg := &imaplib.Config{
		Host:     host,
		Port:     port,
		Username: "testuser@example.com",
		Password: "testpassword",
		UseTLS:   false,
	}

	// GreenMail may not accept logins immediately after the startup log line.
	// Retry with a short backoff to handle this CI timing window.
	var client *imapclient.Client
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		client, err = imapDial(cfg)
		if err == nil {
			break
		}
		t.Logf("imapDial attempt %d/5 failed: %v", attempt+1, err)
	}
	if err != nil {
		t.Fatalf("imapDial() error after retries: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("client.Close() error: %v", err)
		}
	}()

	// List mailboxes and assert INBOX is present.
	mailboxes, err := client.List("", "*", nil).Collect()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	found := false
	names := make([]string, 0, len(mailboxes))
	for _, mb := range mailboxes {
		names = append(names, mb.Mailbox)
		if strings.EqualFold(mb.Mailbox, "INBOX") {
			found = true
		}
	}
	if !found {
		t.Errorf("INBOX not found in mailbox list; got: %v", names)
	}
}

func TestParseIMAPCredential(t *testing.T) {
	t.Run("valid credential with all fields", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host":     "imap.example.com",
				"port":     "993",
				"username": "user@example.com",
				"password": "secret",
				"use_tls":  "true",
			},
		}
		cfg, err := parseIMAPCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Host != "imap.example.com" {
			t.Errorf("Host = %q, want %q", cfg.Host, "imap.example.com")
		}
		if cfg.Port != "993" {
			t.Errorf("Port = %q, want %q", cfg.Port, "993")
		}
		if cfg.Username != "user@example.com" {
			t.Errorf("Username = %q, want %q", cfg.Username, "user@example.com")
		}
		if cfg.Password != "secret" {
			t.Errorf("Password = %q, want %q", cfg.Password, "secret")
		}
		if !cfg.UseTLS {
			t.Error("UseTLS = false, want true")
		}
	})

	t.Run("missing credential key", func(t *testing.T) {
		params := map[string]any{
			"host": "imap.example.com",
		}
		_, err := parseIMAPCredential(params)
		if err == nil {
			t.Fatal("expected error for missing _credential, got nil")
		}
	})

	t.Run("missing host", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"username": "user@example.com",
				"password": "secret",
			},
		}
		_, err := parseIMAPCredential(params)
		if err == nil {
			t.Fatal("expected error for missing host, got nil")
		}
		if !strings.Contains(err.Error(), "host") {
			t.Errorf("error %q should mention 'host'", err.Error())
		}
	})

	t.Run("defaults to port 993 and TLS true", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host":     "imap.example.com",
				"username": "user@example.com",
				"password": "secret",
			},
		}
		cfg, err := parseIMAPCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Port != "993" {
			t.Errorf("Port = %q, want default %q", cfg.Port, "993")
		}
		if !cfg.UseTLS {
			t.Error("UseTLS = false, want default true")
		}
	})

	t.Run("use_tls=false disables TLS", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host":     "imap.example.com",
				"username": "user",
				"password": "pass",
				"use_tls":  "false",
			},
		}
		cfg, err := parseIMAPCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.UseTLS {
			t.Error("UseTLS = true, want false")
		}
	})

	t.Run("wrong credential type", func(t *testing.T) {
		params := map[string]any{
			"_credential": "not-a-map",
		}
		_, err := parseIMAPCredential(params)
		if err == nil {
			t.Fatal("expected error for wrong credential type, got nil")
		}
	})

	t.Run("explicit port is preserved", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host":     "imap.example.com",
				"port":     "143",
				"username": "user",
				"password": "pass",
			},
		}
		cfg, err := parseIMAPCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Port != "143" {
			t.Errorf("Port = %q, want %q", cfg.Port, "143")
		}
	})
}
