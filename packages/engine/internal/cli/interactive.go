package cli

import (
	"os"

	"golang.org/x/term"
)

// isInteractive returns true if stdin is a real terminal (not piped or redirected).
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
