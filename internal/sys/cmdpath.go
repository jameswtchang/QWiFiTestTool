package sys

import (
	"fmt"
	"os/exec"
)

// CommandPath finds the absolute path of a command using exec.LookPath
// Returns the path or error if not found
func CommandPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("command '%s' not found in PATH: %w", name, err)
	}
	return path, nil
}

// FindCommand finds a command and returns its path, or a fallback if not found
func FindCommand(name string, fallback string) string {
	if path, err := CommandPath(name); err == nil {
		return path
	}
	// Return fallback if command not found
	return fallback
}

// FindCommandOrEmpty finds a command path, returns empty string if not found
func FindCommandOrEmpty(name string) string {
	path, _ := CommandPath(name)
	return path
}
