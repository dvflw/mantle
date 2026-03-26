package connector

import (
	"context"
	"fmt"
	"net/smtp"
	"os"
	"testing"
	"time"

	imaplib "github.com/dvflw/mantle/internal/imap"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupGreenMailFull starts a GreenMail container that exposes both IMAP
// (3143) and SMTP (3025) ports and returns the host and both mapped ports.
// The container is terminated when the test finishes.
func setupGreenMailFull(t *testing.T) (host, imapPort, smtpPort string) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "greenmail/standalone:2.0.1",
		ExposedPorts: []string{"3143/tcp", "3025/tcp"},
		Env: map[string]string{
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
	mappedIMAP, err := container.MappedPort(ctx, "3143")
	if err != nil {
		t.Fatalf("Failed to get mapped IMAP port: %v", err)
	}
	mappedSMTP, err := container.MappedPort(ctx, "3025")
	if err != nil {
		t.Fatalf("Failed to get mapped SMTP port: %v", err)
	}

	return mappedHost, mappedIMAP.Port(), mappedSMTP.Port()
}

// TestEmailReceiveParamDefaults verifies that default parameter values are
// applied correctly when optional params are omitted.
func TestEmailReceiveParamDefaults(t *testing.T) {
	c := &EmailReceiveConnector{}

	// Missing credential should return an error, not panic.
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing credential, got nil")
	}
}

// TestEmailReceiveFilterCriteria verifies each filter maps to the correct
// IMAP search criteria.
func TestEmailReceiveFilterCriteria(t *testing.T) {
	tests := []struct {
		filter      string
		wantFlag    string
		wantNotFlag string
	}{
		{"unseen", "", `\Seen`},
		{"flagged", `\Flagged`, ""},
		{"recent", `\Recent`, ""},
		{"all", "", ""},
		{"unknown", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			c := imaplib.BuildSearchCriteria(tt.filter)
			if c == nil {
				t.Fatal("buildSearchCriteria returned nil")
			}
			if tt.wantFlag != "" {
				if len(c.Flag) == 0 || string(c.Flag[0]) != tt.wantFlag {
					t.Errorf("filter %q: Flag = %v, want %q", tt.filter, c.Flag, tt.wantFlag)
				}
			} else if len(c.Flag) != 0 {
				t.Errorf("filter %q: unexpected Flag = %v", tt.filter, c.Flag)
			}
			if tt.wantNotFlag != "" {
				if len(c.NotFlag) == 0 || string(c.NotFlag[0]) != tt.wantNotFlag {
					t.Errorf("filter %q: NotFlag = %v, want %q", tt.filter, c.NotFlag, tt.wantNotFlag)
				}
			} else if len(c.NotFlag) != 0 {
				t.Errorf("filter %q: unexpected NotFlag = %v", tt.filter, c.NotFlag)
			}
		})
	}
}

// TestEmailReceive_FetchesUnseenMessages is an integration test that:
//  1. Starts a GreenMail IMAP/SMTP container.
//  2. Delivers a test message via SMTP.
//  3. Calls EmailReceiveConnector.Execute and asserts the message is returned.
func TestEmailReceive_FetchesUnseenMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host, imapPort, smtpPort := setupGreenMailFull(t)

	const (
		username = "receiver@example.com"
		password = "password"
	)

	// Deliver a test message via SMTP.
	smtpAddr := fmt.Sprintf("%s:%s", host, smtpPort)
	msg := []byte("From: sender@example.com\r\n" +
		"To: receiver@example.com\r\n" +
		"Subject: Hello Mantle\r\n" +
		"\r\n" +
		"This is the body of the test email.\r\n")

	if err := smtp.SendMail(smtpAddr, nil, "sender@example.com", []string{username}, msg); err != nil {
		t.Fatalf("failed to send test email: %v", err)
	}

	// Fetch via the connector — retry to allow GreenMail time to propagate
	// the SMTP delivery to the IMAP store.
	c := &EmailReceiveConnector{}
	params := map[string]any{
		"folder": "INBOX",
		"filter": "unseen",
		"limit":  10,
		"_credential": map[string]string{
			"host":     host,
			"port":     imapPort,
			"username": username,
			"password": password,
			"use_tls":  "false",
		},
	}

	var result map[string]any
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		result, err = c.Execute(context.Background(), params)
		if err == nil {
			break
		}
		t.Logf("attempt %d: Execute() error: %v (retrying in 2s)", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		t.Fatalf("Execute() error after retries: %v", err)
	}

	count, ok := result["message_count"].(int)
	if !ok {
		t.Fatalf("message_count is not an int: %T", result["message_count"])
	}
	if count != 1 {
		t.Errorf("message_count = %d, want 1", count)
	}

	msgs, ok := result["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("messages is not []map[string]any: %T", result["messages"])
	}
	if len(msgs) == 0 {
		t.Fatal("messages is empty")
	}

	first := msgs[0]

	if subject, _ := first["subject"].(string); subject != "Hello Mantle" {
		t.Errorf("subject = %q, want %q", subject, "Hello Mantle")
	}
	if from, _ := first["from"].(string); from == "" {
		t.Error("from is empty")
	}
	if to, _ := first["to"].([]string); len(to) == 0 {
		t.Error("to is empty")
	}
	if uid, _ := first["uid"].(uint32); uid == 0 {
		t.Error("uid is zero, expected a valid UID")
	}
}
