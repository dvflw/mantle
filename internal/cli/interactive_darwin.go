//go:build darwin

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// isInteractive returns true if stdin is a real terminal (not piped or redirected).
//
// It uses the TIOCGETA ioctl rather than a simple ModeCharDevice check because
// go test on macOS sets stdin to /dev/null, which reports as a char device but
// is not a terminal. The ioctl succeeds only on real TTYs.
func isInteractive() bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TIOCGETA, uintptr(unsafe.Pointer(&termios)))
	return errno == 0
}
