//go:build !linux

package sys

import "os/exec"

// Detach is a no-op off Linux/Android; the tool only manages daemons on those platforms.
func Detach(cmd *exec.Cmd) {}
