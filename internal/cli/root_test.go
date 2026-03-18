package cli

import (
	"bytes"
	"testing"
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
