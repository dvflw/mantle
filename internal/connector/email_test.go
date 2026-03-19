package connector

import (
	"context"
	"strings"
	"testing"
)

func TestEmailSend_MissingTo(t *testing.T) {
	c := &EmailSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"from":    "sender@example.com",
		"subject": "Test",
		"body":    "Hello",
		"_credential": map[string]string{
			"host": "smtp.example.com", "port": "587",
			"username": "u", "password": "p",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing to")
	}
	if !strings.Contains(err.Error(), "to is required") {
		t.Errorf("error = %v, want 'to is required'", err)
	}
}

func TestEmailSend_MissingFrom(t *testing.T) {
	c := &EmailSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"to":      "recipient@example.com",
		"subject": "Test",
		"body":    "Hello",
		"_credential": map[string]string{
			"host": "smtp.example.com", "port": "587",
			"username": "u", "password": "p",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing from")
	}
	if !strings.Contains(err.Error(), "from is required") {
		t.Errorf("error = %v, want 'from is required'", err)
	}
}

func TestEmailSend_MissingSubject(t *testing.T) {
	c := &EmailSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"to":   "recipient@example.com",
		"from": "sender@example.com",
		"body": "Hello",
		"_credential": map[string]string{
			"host": "smtp.example.com", "port": "587",
			"username": "u", "password": "p",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing subject")
	}
	if !strings.Contains(err.Error(), "subject is required") {
		t.Errorf("error = %v, want 'subject is required'", err)
	}
}

func TestEmailSend_MissingBody(t *testing.T) {
	c := &EmailSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"to":      "recipient@example.com",
		"from":    "sender@example.com",
		"subject": "Test",
		"_credential": map[string]string{
			"host": "smtp.example.com", "port": "587",
			"username": "u", "password": "p",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing body")
	}
	if !strings.Contains(err.Error(), "body is required") {
		t.Errorf("error = %v, want 'body is required'", err)
	}
}

func TestEmailSend_MissingHost(t *testing.T) {
	c := &EmailSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"to":      "recipient@example.com",
		"from":    "sender@example.com",
		"subject": "Test",
		"body":    "Hello",
	})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
	if !strings.Contains(err.Error(), "smtp host is required") {
		t.Errorf("error = %v, want 'smtp host is required'", err)
	}
}

func TestEmailSend_HostFromParams(t *testing.T) {
	c := &EmailSendConnector{}
	// This will fail at the SMTP connection stage, but should get past validation.
	_, err := c.Execute(context.Background(), map[string]any{
		"to":        "recipient@example.com",
		"from":      "sender@example.com",
		"subject":   "Test",
		"body":      "Hello",
		"smtp_host": "localhost",
		"smtp_port": "19999",
	})
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
	// Should be a connection error, not a validation error.
	if strings.Contains(err.Error(), "is required") {
		t.Errorf("error = %v, want connection error not validation error", err)
	}
}

func TestBuildMessage_PlainText(t *testing.T) {
	msg := buildMessage(
		"sender@example.com",
		[]string{"a@example.com", "b@example.com"},
		"Test Subject",
		"Hello, World!",
		false,
	)
	s := string(msg)

	checks := []struct {
		label    string
		expected string
	}{
		{"From header", "From: sender@example.com\r\n"},
		{"To header", "To: a@example.com, b@example.com\r\n"},
		{"Subject header", "Subject: Test Subject\r\n"},
		{"MIME version", "MIME-Version: 1.0\r\n"},
		{"Content-Type", "Content-Type: text/plain; charset=\"UTF-8\"\r\n"},
		{"Body", "\r\nHello, World!"},
	}
	for _, c := range checks {
		if !strings.Contains(s, c.expected) {
			t.Errorf("%s: message does not contain %q\nmessage:\n%s", c.label, c.expected, s)
		}
	}
}

func TestBuildMessage_HTML(t *testing.T) {
	msg := buildMessage(
		"sender@example.com",
		[]string{"a@example.com"},
		"HTML Email",
		"<h1>Hello</h1>",
		true,
	)
	s := string(msg)

	if !strings.Contains(s, "Content-Type: text/html; charset=\"UTF-8\"") {
		t.Errorf("expected text/html content type\nmessage:\n%s", s)
	}
	if !strings.Contains(s, "<h1>Hello</h1>") {
		t.Errorf("expected HTML body in message\nmessage:\n%s", s)
	}
}

func TestParseRecipients_String(t *testing.T) {
	r, err := parseRecipients("a@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r) != 1 || r[0] != "a@example.com" {
		t.Errorf("got %v, want [a@example.com]", r)
	}
}

func TestParseRecipients_Slice(t *testing.T) {
	r, err := parseRecipients([]any{"a@example.com", "b@example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r) != 2 {
		t.Errorf("got %d recipients, want 2", len(r))
	}
}

func TestParseRecipients_StringSlice(t *testing.T) {
	r, err := parseRecipients([]string{"a@example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r) != 1 || r[0] != "a@example.com" {
		t.Errorf("got %v, want [a@example.com]", r)
	}
}

func TestParseRecipients_EmptyString(t *testing.T) {
	_, err := parseRecipients("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestParseRecipients_EmptySlice(t *testing.T) {
	_, err := parseRecipients([]any{})
	if err == nil {
		t.Fatal("expected error for empty slice")
	}
}

func TestParseRecipients_Nil(t *testing.T) {
	_, err := parseRecipients(nil)
	if err == nil {
		t.Fatal("expected error for nil")
	}
}

func TestParseRecipients_InvalidType(t *testing.T) {
	_, err := parseRecipients(42)
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestParseRecipients_SliceWithEmpty(t *testing.T) {
	_, err := parseRecipients([]any{"a@example.com", ""})
	if err == nil {
		t.Fatal("expected error for slice containing empty string")
	}
}

func TestEmailSend_CredentialDeleted(t *testing.T) {
	// Verify that _credential is removed from params after extraction.
	params := map[string]any{
		"to":      "recipient@example.com",
		"from":    "sender@example.com",
		"subject": "Test",
		"body":    "Hello",
		"_credential": map[string]string{
			"host": "localhost", "port": "19999",
			"username": "u", "password": "p",
		},
	}

	c := &EmailSendConnector{}
	// Will fail at SMTP connection, but credential should still be deleted.
	_, _ = c.Execute(context.Background(), params)

	if _, ok := params["_credential"]; ok {
		t.Error("_credential should be deleted from params after extraction")
	}
}

func TestRegistry_EmailSendRegistered(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("email/send")
	if err != nil {
		t.Fatalf("Get(email/send) error: %v", err)
	}
}
