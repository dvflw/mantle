package connector

import (
	"context"
	"fmt"
	"net/smtp"
	"testing"
)

// TestEmailDeleteParamValidation verifies that required parameters are enforced.
func TestEmailDeleteParamValidation(t *testing.T) {
	c := &EmailDeleteConnector{}

	t.Run("missing credential", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid": uint32(1),
		})
		if err == nil {
			t.Fatal("expected error for missing credential, got nil")
		}
	})

	t.Run("missing uid", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for missing uid, got nil")
		}
	})

	t.Run("uid zero", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid": uint32(0),
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for uid=0, got nil")
		}
	})

	t.Run("uid as float64", func(t *testing.T) {
		// Float64 is the type used by JSON unmarshalling; validate it is handled.
		_, err := c.Execute(context.Background(), map[string]any{
			"uid": float64(0),
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for uid=0.0, got nil")
		}
	})
}

// TestEmailDelete_DeletesMessage is an integration test that:
//  1. Starts a GreenMail IMAP/SMTP container.
//  2. Delivers a test message via SMTP.
//  3. Calls EmailDeleteConnector.Execute to delete it.
//  4. Verifies that the message is gone from the folder.
func TestEmailDelete_DeletesMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host, imapPort, smtpPort := setupGreenMailFull(t)

	const (
		username = "deleter@example.com"
		password = "password"
	)

	// Deliver a test message via SMTP.
	smtpAddr := fmt.Sprintf("%s:%s", host, smtpPort)
	msg := []byte("From: sender@example.com\r\n" +
		"To: deleter@example.com\r\n" +
		"Subject: Delete Me\r\n" +
		"\r\n" +
		"Please delete this message.\r\n")

	if err := smtp.SendMail(smtpAddr, nil, "sender@example.com", []string{username}, msg); err != nil {
		t.Fatalf("failed to send test email: %v", err)
	}

	// Fetch the UID of the delivered message.
	uid := fetchFirstUID(t, host, imapPort, username, password, "INBOX")

	// Delete the message.
	c := &EmailDeleteConnector{}
	result, err := c.Execute(context.Background(), map[string]any{
		"uid":    uid,
		"folder": "INBOX",
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

	if deleted, _ := result["deleted"].(bool); !deleted {
		t.Errorf("deleted = %v, want true", result["deleted"])
	}
	if retUID, _ := result["uid"].(uint32); retUID != uid {
		t.Errorf("uid = %v, want %d", result["uid"], uid)
	}

	// Verify the message is no longer in INBOX.
	recv := &EmailReceiveConnector{}
	checkResult, err := recv.Execute(context.Background(), map[string]any{
		"folder": "INBOX",
		"filter": "all",
		"limit":  10,
		"_credential": map[string]string{
			"host":     host,
			"port":     imapPort,
			"username": username,
			"password": password,
			"use_tls":  "false",
		},
	})
	if err != nil {
		t.Fatalf("post-delete receive error: %v", err)
	}
	if count, _ := checkResult["message_count"].(int); count != 0 {
		t.Errorf("message_count after delete = %d, want 0", count)
	}
}
