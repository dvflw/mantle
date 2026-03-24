//go:build windows

package cli

import (
	"os"
)

// isInteractive returns true if stdin is a real terminal on Windows.
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
