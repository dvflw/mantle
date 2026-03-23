package version

import (
	"fmt"
	"runtime"
)

// Set via ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a formatted version string.
func String() string {
	return fmt.Sprintf("mantle %s (%s, built %s, %s/%s)", Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
}
