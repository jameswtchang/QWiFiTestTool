package sys

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// standardSystemDirs lists common directories where executable utilities reside on Linux/Android
var standardSystemDirs = []string{
	"/usr/sbin",
	"/sbin",
	"/usr/local/sbin",
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
	"/system/bin",
	"/system/xbin",
	"/vendor/bin",
	"/data/local/tmp",
}

// CommandPath finds the absolute path of a command using exec.LookPath
// Returns the path or error if not found
func CommandPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("command '%s' not found in PATH: %w", name, err)
	}
	return path, nil
}

// fileExistsAndExecutable checks if path exists and is not a directory
func fileExistsAndExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return true
}

// FindCommand finds a command and returns its absolute path, or a fallback if not found.
// Search strategy:
// 1. exec.LookPath (in current $PATH)
// 2. Directory containing current running executable
// 3. Standard system directories (/usr/sbin, /sbin, /usr/local/bin, /system/bin, /data/local/tmp, etc.)
// 4. Fallback path if it exists on filesystem
// 5. If fallback is a bare filename, check standard directories for that fallback name
// 6. Return fallback
func FindCommand(name string, fallback string) string {
	// 1. Check in $PATH
	if path, err := CommandPath(name); err == nil {
		return path
	}

	// 2. Check next to current running executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidate := filepath.Join(exeDir, name)
		if fileExistsAndExecutable(candidate) {
			return candidate
		}
	}

	// 3. Check standard system directories
	for _, dir := range standardSystemDirs {
		candidate := filepath.Join(dir, name)
		if fileExistsAndExecutable(candidate) {
			return candidate
		}
	}

	// 4. Check if fallback exists directly
	if fallback != "" && fileExistsAndExecutable(fallback) {
		return fallback
	}

	// 5. If fallback is a bare name without path separators, search standard directories for fallback
	if fallback != "" && filepath.Base(fallback) == fallback && fallback != name {
		for _, dir := range standardSystemDirs {
			candidate := filepath.Join(dir, fallback)
			if fileExistsAndExecutable(candidate) {
				return candidate
			}
		}
	}

	// Return fallback if command not found
	return fallback
}

// FindCommandOrEmpty finds a command path, returns empty string if not found
func FindCommandOrEmpty(name string) string {
	path := FindCommand(name, "")
	return path
}
