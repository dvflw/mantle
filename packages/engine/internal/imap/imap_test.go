package imap

import (
	"testing"

	goimap "github.com/emersion/go-imap/v2"
)

func TestParseCredential(t *testing.T) {
	t.Run("valid credential", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host":     "mail.example.com",
				"port":     "993",
				"username": "user@example.com",
				"password": "secret",
				"use_tls":  "true",
			},
		}
		cfg, err := ParseCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Host != "mail.example.com" {
			t.Errorf("Host = %q, want %q", cfg.Host, "mail.example.com")
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
		params := map[string]any{}
		_, err := ParseCredential(params)
		if err == nil {
			t.Fatal("expected error for missing credential")
		}
	})

	t.Run("wrong credential type", func(t *testing.T) {
		params := map[string]any{
			"_credential": "not-a-map",
		}
		_, err := ParseCredential(params)
		if err == nil {
			t.Fatal("expected error for wrong credential type")
		}
	})

	t.Run("missing host", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"username": "user@example.com",
				"password": "secret",
			},
		}
		_, err := ParseCredential(params)
		if err == nil {
			t.Fatal("expected error for missing host")
		}
	})

	t.Run("default port", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host":     "mail.example.com",
				"username": "user@example.com",
				"password": "secret",
			},
		}
		cfg, err := ParseCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Port != "993" {
			t.Errorf("Port = %q, want default %q", cfg.Port, "993")
		}
	})

	t.Run("use_tls false", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host":    "mail.example.com",
				"use_tls": "false",
			},
		}
		cfg, err := ParseCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.UseTLS {
			t.Error("UseTLS = true, want false")
		}
	})

	t.Run("use_tls defaults to true when absent", func(t *testing.T) {
		params := map[string]any{
			"_credential": map[string]string{
				"host": "mail.example.com",
			},
		}
		cfg, err := ParseCredential(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.UseTLS {
			t.Error("UseTLS = false, want true (default)")
		}
	})
}

func TestParseCredentialMap(t *testing.T) {
	t.Run("valid map", func(t *testing.T) {
		cred := map[string]string{
			"host":     "imap.test.com",
			"port":     "143",
			"username": "tester",
			"password": "pw",
			"use_tls":  "false",
		}
		cfg, err := ParseCredentialMap(cred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Host != "imap.test.com" {
			t.Errorf("Host = %q, want %q", cfg.Host, "imap.test.com")
		}
		if cfg.Port != "143" {
			t.Errorf("Port = %q, want %q", cfg.Port, "143")
		}
		if cfg.UseTLS {
			t.Error("UseTLS = true, want false")
		}
	})

	t.Run("missing host", func(t *testing.T) {
		cred := map[string]string{
			"username": "tester",
		}
		_, err := ParseCredentialMap(cred)
		if err == nil {
			t.Fatal("expected error for missing host")
		}
	})
}

func TestBuildSearchCriteria(t *testing.T) {
	tests := []struct {
		filter      string
		wantNotFlag []goimap.Flag
		wantFlag    []goimap.Flag
	}{
		{
			filter:      "unseen",
			wantNotFlag: []goimap.Flag{goimap.FlagSeen},
		},
		{
			filter:   "flagged",
			wantFlag: []goimap.Flag{goimap.FlagFlagged},
		},
		{
			filter:   "recent",
			wantFlag: []goimap.Flag{goimap.Flag(`\Recent`)},
		},
		{
			filter: "all",
		},
		{
			filter: "unknown-defaults-to-all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			c := BuildSearchCriteria(tt.filter)

			if len(c.NotFlag) != len(tt.wantNotFlag) {
				t.Fatalf("NotFlag length = %d, want %d", len(c.NotFlag), len(tt.wantNotFlag))
			}
			for i, f := range c.NotFlag {
				if f != tt.wantNotFlag[i] {
					t.Errorf("NotFlag[%d] = %q, want %q", i, f, tt.wantNotFlag[i])
				}
			}

			if len(c.Flag) != len(tt.wantFlag) {
				t.Fatalf("Flag length = %d, want %d", len(c.Flag), len(tt.wantFlag))
			}
			for i, f := range c.Flag {
				if f != tt.wantFlag[i] {
					t.Errorf("Flag[%d] = %q, want %q", i, f, tt.wantFlag[i])
				}
			}
		})
	}
}
