package connector

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// TestEmailFlagParamValidation verifies that required parameters are enforced.
func TestEmailFlagParamValidation(t *testing.T) {
	c := &EmailFlagConnector{}

	t.Run("missing credential", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid":    uint32(1),
			"flags":  []string{"seen"},
			"action": "add",
		})
		if err == nil {
			t.Fatal("expected error for missing credential, got nil")
		}
	})

	t.Run("missing uid", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"flags":  []string{"seen"},
			"action": "add",
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for missing uid, got nil")
		}
	})

	t.Run("missing flags", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid":    uint32(1),
			"action": "add",
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for missing flags, got nil")
		}
	})

	t.Run("empty flags", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid":    uint32(1),
			"flags":  []string{},
			"action": "add",
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for empty flags, got nil")
		}
	})

	t.Run("missing action", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid":   uint32(1),
			"flags": []string{"seen"},
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for missing action, got nil")
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		_, err := c.Execute(context.Background(), map[string]any{
			"uid":    uint32(1),
			"flags":  []string{"seen"},
			"action": "set",
			"_credential": map[string]string{
				"host": "localhost", "port": "993",
				"username": "u", "password": "p",
			},
		})
		if err == nil {
			t.Fatal("expected error for invalid action, got nil")
		}
		if !strings.Contains(err.Error(), "\"add\" or \"remove\"") {
			t.Errorf("error = %v, want mention of add/remove", err)
		}
	})
}

// TestMapFlagNames verifies that flag names are correctly mapped to IMAP flag constants.
func TestMapFlagNames(t *testing.T) {
	tests := []struct {
		input    string
		wantFlag imap.Flag
	}{
		{"seen", imap.FlagSeen},
		{"SEEN", imap.FlagSeen},
		{"flagged", imap.FlagFlagged},
		{"answered", imap.FlagAnswered},
		{"deleted", imap.FlagDeleted},
		{"draft", imap.FlagDraft},
		{"custom-tag", imap.Flag("custom-tag")},
		{"MyKeyword", imap.Flag("MyKeyword")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			flags := mapFlagNames([]string{tt.input})
			if len(flags) != 1 {
				t.Fatalf("mapFlagNames(%q) returned %d flags, want 1", tt.input, len(flags))
			}
			if flags[0] != tt.wantFlag {
				t.Errorf("mapFlagNames(%q) = %q, want %q", tt.input, flags[0], tt.wantFlag)
			}
		})
	}
}

// TestExtractFlagNames verifies that various input types are handled correctly.
func TestExtractFlagNames(t *testing.T) {
	t.Run("string slice", func(t *testing.T) {
		flags, err := extractFlagNames(map[string]any{"flags": []string{"seen", "flagged"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(flags) != 2 {
			t.Errorf("got %d flags, want 2", len(flags))
		}
	})

	t.Run("any slice", func(t *testing.T) {
		flags, err := extractFlagNames(map[string]any{"flags": []any{"seen", "custom"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(flags) != 2 {
			t.Errorf("got %d flags, want 2", len(flags))
		}
	})

	t.Run("nil", func(t *testing.T) {
		_, err := extractFlagNames(map[string]any{"flags": nil})
		if err == nil {
			t.Fatal("expected error for nil flags")
		}
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := extractFlagNames(map[string]any{})
		if err == nil {
			t.Fatal("expected error for missing flags key")
		}
	})

	t.Run("empty any slice", func(t *testing.T) {
		_, err := extractFlagNames(map[string]any{"flags": []any{}})
		if err == nil {
			t.Fatal("expected error for empty flags slice")
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		_, err := extractFlagNames(map[string]any{"flags": "seen"})
		if err == nil {
			t.Fatal("expected error for string instead of slice")
		}
	})
}

// TestEmailFlag_AddAndRemoveFlag is an integration test that:
//  1. Starts a GreenMail IMAP/SMTP container.
//  2. Delivers a test message via SMTP.
//  3. Adds the \Seen flag and verifies.
//  4. Removes the \Seen flag and verifies.
func TestEmailFlag_AddAndRemoveFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host, imapPort, smtpPort := setupGreenMailFull(t)

	const (
		username = "flagger@example.com"
		password = "password"
	)

	// Deliver a test message via SMTP.
	smtpAddr := fmt.Sprintf("%s:%s", host, smtpPort)
	msg := []byte("From: sender@example.com\r\n" +
		"To: flagger@example.com\r\n" +
		"Subject: Flag Me\r\n" +
		"\r\n" +
		"Please flag this message.\r\n")

	if err := smtp.SendMail(smtpAddr, nil, "sender@example.com", []string{username}, msg); err != nil {
		t.Fatalf("failed to send test email: %v", err)
	}

	// Fetch the UID of the delivered message.
	uid := fetchFirstUID(t, host, imapPort, username, password, "INBOX")

	cred := map[string]string{
		"host":     host,
		"port":     imapPort,
		"username": username,
		"password": password,
		"use_tls":  "false",
	}

	// Add \Seen flag.
	fc := &EmailFlagConnector{}
	addResult, err := fc.Execute(context.Background(), map[string]any{
		"uid":         uid,
		"folder":      "INBOX",
		"flags":       []string{"seen"},
		"action":      "add",
		"_credential": cred,
	})
	if err != nil {
		t.Fatalf("Execute() add error: %v", err)
	}

	if updated, _ := addResult["updated"].(bool); !updated {
		t.Errorf("add: updated = %v, want true", addResult["updated"])
	}
	if retUID, _ := addResult["uid"].(uint32); retUID != uid {
		t.Errorf("add: uid = %v, want %d", addResult["uid"], uid)
	}
	if action, _ := addResult["action"].(string); action != "add" {
		t.Errorf("add: action = %q, want \"add\"", action)
	}

	// Verify the flag was applied: receive with filter "unseen" should return 0 messages.
	recv := &EmailReceiveConnector{}
	unseenResult, err := recv.Execute(context.Background(), map[string]any{
		"folder":      "INBOX",
		"filter":      "unseen",
		"limit":       10,
		"_credential": cred,
	})
	if err != nil {
		t.Fatalf("receive (unseen) error: %v", err)
	}
	if count, _ := unseenResult["message_count"].(int); count != 0 {
		t.Errorf("after add seen: unseen count = %d, want 0", count)
	}

	// Remove \Seen flag.
	removeResult, err := fc.Execute(context.Background(), map[string]any{
		"uid":         uid,
		"folder":      "INBOX",
		"flags":       []string{"seen"},
		"action":      "remove",
		"_credential": cred,
	})
	if err != nil {
		t.Fatalf("Execute() remove error: %v", err)
	}

	if updated, _ := removeResult["updated"].(bool); !updated {
		t.Errorf("remove: updated = %v, want true", removeResult["updated"])
	}
	if action, _ := removeResult["action"].(string); action != "remove" {
		t.Errorf("remove: action = %q, want \"remove\"", action)
	}

	// Verify the flag was removed: receive with filter "unseen" should return 1 message.
	unseenResult2, err := recv.Execute(context.Background(), map[string]any{
		"folder":      "INBOX",
		"filter":      "unseen",
		"limit":       10,
		"_credential": cred,
	})
	if err != nil {
		t.Fatalf("receive (unseen after remove) error: %v", err)
	}
	if count, _ := unseenResult2["message_count"].(int); count != 1 {
		t.Errorf("after remove seen: unseen count = %d, want 1", count)
	}
}
