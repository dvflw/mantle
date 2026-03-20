//go:build windows

package cli

import "os/exec"

func browserCommand(url string) *exec.Cmd { return exec.Command("cmd", "/c", "start", url) }
