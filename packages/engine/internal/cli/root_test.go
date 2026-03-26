package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dvflw/mantle/internal/config"
	"github.com/spf13/cobra"
)

func TestRootCommand_NoArgs(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("expected help output, got empty string")
	}
}

func TestRootCommand_ConfigLoaded(t *testing.T) {
	cmd := NewRootCommand()
	var gotConfig *config.Config
	cmd.AddCommand(&cobra.Command{
		Use: "testcfg",
		RunE: func(cmd *cobra.Command, args []string) error {
			gotConfig = config.FromContext(cmd.Context())
			return nil
		},
	})

	cmd.SetArgs([]string{"testcfg"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotConfig == nil {
		t.Fatal("expected config on context, got nil")
	}
	if gotConfig.API.Address != ":8080" {
		t.Errorf("API.Address = %q, want default :8080", gotConfig.API.Address)
	}
}

func TestRootCommand_ConfigFlagOverride(t *testing.T) {
	cmd := NewRootCommand()
	var gotConfig *config.Config
	cmd.AddCommand(&cobra.Command{
		Use: "testcfg",
		RunE: func(cmd *cobra.Command, args []string) error {
			gotConfig = config.FromContext(cmd.Context())
			return nil
		},
	})

	cmd.SetArgs([]string{"testcfg", "--log-level", "error"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotConfig == nil {
		t.Fatal("expected config on context, got nil")
	}
	if gotConfig.Log.Level != "error" {
		t.Errorf("Log.Level = %q, want error", gotConfig.Log.Level)
	}
}

func TestVersionCommand_WorksWithoutConfig(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version", "--config", "/nonexistent/mantle.yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version should work even with invalid config, got error = %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "mantle ") {
		t.Errorf("expected version output, got %q", output)
	}
}
