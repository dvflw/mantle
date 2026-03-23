package version

import (
	"runtime"
	"testing"
)

func TestString_Defaults(t *testing.T) {
	got := String()
	want := "mantle dev (none, built unknown, " + runtime.GOOS + "/" + runtime.GOARCH + ")"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestString_WithValues(t *testing.T) {
	origVersion, origCommit, origDate := Version, Commit, Date
	Version = "v0.1.0"
	Commit = "abc1234"
	Date = "2026-03-18T15:30:00Z"
	defer func() {
		Version, Commit, Date = origVersion, origCommit, origDate
	}()

	got := String()
	want := "mantle v0.1.0 (abc1234, built 2026-03-18T15:30:00Z, " + runtime.GOOS + "/" + runtime.GOARCH + ")"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
