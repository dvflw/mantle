//go:build linux

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// isInteractive returns true if stdin is a real terminal (not piped or redirected).
//
// It uses the TCGETS ioctl rather than a simple ModeCharDevice check to
// correctly identify real TTYs vs. devices like /dev/null.
func isInteractive() bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&termios)))
	return errno == 0
}
