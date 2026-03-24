package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/dvflw/mantle/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestIsInteractive_ReturnsBool(t *testing.T) {
	// In test context, stdin is not a TTY — isInteractive should return false.
	assert.False(t, isInteractive())
}

func TestHandleConnectionFailure_NonInteractive_ReturnsError(t *testing.T) {
	// When stdin is not a TTY, handleConnectionFailure should return the
	// connection error immediately without prompting.
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cfg := &config.Config{}
	cfg.Database.URL = "postgres://mantle:mantle@localhost:5432/mantle"

	_, err := handleConnectionFailure(cmd, cfg, fmt.Errorf("connection refused"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	// No prompt text should have been written to stdout.
	assert.Empty(t, buf.String())
}
