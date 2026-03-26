package connector

import (
	"context"
	"fmt"
	"net/smtp"
	"testing"
	"time"

	imaplib "github.com/dvflw/mantle/internal/imap"
)

// TestEmailMoveParamValidation verifies that required parameters are enforced.
func TestEmailMoveParamValidation(t *testing.T) {
	c := &EmailMoveConnector{}

	t.Run("missing credential", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid":           uint32(1),
			"target_folder": "Archive",
		})
		if err == nil {
			t.Fatal("expected error for missing credential, got nil")
		}
	})

	t.Run("missing uid", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"target_folder": "Archive",
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for missing uid, got nil")
		}
	})

	t.Run("missing target_folder", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid": uint32(1),
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for missing target_folder, got nil")
		}
	})

	t.Run("uid zero", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid":           uint32(0),
			"target_folder": "Archive",
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for uid=0, got nil")
		}
	})
}

// TestEmailMove_MovesMessage is an integration test that:
//  1. Starts a GreenMail IMAP/SMTP container.
//  2. Delivers a test message via SMTP.
//  3. Calls EmailMoveConnector.Execute to move it to a target folder.
//  4. Verifies the move result.
func TestEmailMove_MovesMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host, imapPort, smtpPort := setupGreenMailFull(t)

	const (
		username = "mover@example.com"
		password = "password"
	)

	// Deliver a test message via SMTP.
	smtpAddr := fmt.Sprintf("%s:%s", host, smtpPort)
	msg := []byte("From: sender@example.com\r\n" +
		"To: mover@example.com\r\n" +
		"Subject: Move Me\r\n" +
		"\r\n" +
		"Please move this message.\r\n")

	if err := smtp.SendMail(smtpAddr, nil, "sender@example.com", []string{username}, msg); err != nil {
		t.Fatalf("failed to send test email: %v", err)
	}

	// Fetch the UID of the delivered message.
	uid := fetchFirstUID(t, host, imapPort, username, password, "INBOX")

	// Create the target folder before moving (GreenMail does not auto-create folders).
	createTargetFolder(t, host, imapPort, username, password, "INBOX.Archive")

	// Move the message.
	c := &EmailMoveConnector{}
	result, err := c.Execute(context.Background(), map[string]any{
		"uid":           uid,
		"source_folder": "INBOX",
		"target_folder": "INBOX.Archive",
		"_credential": map[string]string{
			"host":     host,
			"port":     imapPort,
			"username": username,
			"password": password,
			"use_tls":  "false",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if moved, _ := result["moved"].(bool); !moved {
		t.Errorf("moved = %v, want true", result["moved"])
	}
	if retUID, _ := result["uid"].(uint32); retUID != uid {
		t.Errorf("uid = %v, want %d", result["uid"], uid)
	}
	if tf, _ := result["target_folder"].(string); tf != "INBOX.Archive" {
		t.Errorf("target_folder = %q, want %q", tf, "INBOX.Archive")
	}
}

// createTargetFolder creates an IMAP folder via a direct IMAP connection.
// Used in tests to ensure the target folder exists before moving messages.
func createTargetFolder(t *testing.T, host, imapPort, username, password, folder string) {
	t.Helper()
	cfg := &imaplib.Config{
		Host:     host,
		Port:     imapPort,
		Username: username,
		Password: password,
		UseTLS:   false,
	}
	client, err := imapDial(cfg)
	if err != nil {
		t.Fatalf("createTargetFolder: dial error: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Create(folder, nil).Wait(); err != nil {
		t.Fatalf("createTargetFolder: CREATE %q error: %v", folder, err)
	}
}

// fetchFirstUID retrieves the first UID from the given IMAP folder using the
// EmailReceiveConnector, to avoid duplicating IMAP client logic in tests.
// It retries up to 5 times with a 2-second pause to accommodate the brief
// delay between SMTP delivery and IMAP visibility in CI.
func fetchFirstUID(t *testing.T, host, imapPort, username, password, folder string) uint32 {
	t.Helper()
	recv := &EmailReceiveConnector{}
	cred := map[string]string{
		"host":     host,
		"port":     imapPort,
		"username": username,
		"password": password,
		"use_tls":  "false",
	}

	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		result, err := recv.Execute(context.Background(), map[string]any{
			"folder":      folder,
			"filter":      "all",
			"limit":       1,
			"_credential": cred,
		})
		if err != nil {
			t.Logf("fetchFirstUID attempt %d/5: receive error: %v", attempt+1, err)
			continue
		}
		msgs, ok := result["messages"].([]map[string]any)
		if !ok || len(msgs) == 0 {
			t.Logf("fetchFirstUID attempt %d/5: no messages yet", attempt+1)
			continue
		}
		uid, _ := msgs[0]["uid"].(uint32)
		if uid == 0 {
			t.Logf("fetchFirstUID attempt %d/5: uid is zero", attempt+1)
			continue
		}
		return uid
	}
	t.Fatal("fetchFirstUID: no messages found after retries")
	return 0
}
