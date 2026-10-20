//go:build linux

package sys

import (
	"os/exec"
	"syscall"
)

// Detach starts cmd in its own session so it is not hit by the terminal's SIGHUP or SIGINT and
// keeps running after the tool exits (e.g. a wpa_supplicant whose connection the user keeps).
// Explicit cleanup (Stop, Ctrl+C handler) still terminates it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
