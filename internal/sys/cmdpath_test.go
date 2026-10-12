package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindCommand_Existing(t *testing.T) {
	// "go" binary should be in PATH during testing
	path := FindCommand("go", "/fallback/go")
	if path == "/fallback/go" {
		t.Log("Note: 'go' not found in PATH or executed in isolated env")
	} else if path == "" {
		t.Error("expected non-empty path for 'go'")
	}
}

func TestFindCommand_Fallback(t *testing.T) {
	fakeName := "nonexistent_command_12345"
	fallback := "/usr/sbin/custom_supplicant"
	path := FindCommand(fakeName, fallback)
	if path != fallback {
		t.Errorf("expected fallback %q, got %q", fallback, path)
	}
}

func TestFindCommand_CandidateSearchDirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_bin_dir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fakeBinName := "test_custom_tool"
	fakeBinPath := filepath.Join(tmpDir, fakeBinName)
	if err := os.WriteFile(fakeBinPath, []byte("#!/bin/sh\necho test\n"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	// Add temp dir to standardSystemDirs temporarily
	origDirs := standardSystemDirs
	standardSystemDirs = append([]string{tmpDir}, standardSystemDirs...)
	defer func() {
		standardSystemDirs = origDirs
	}()

	resolved := FindCommand(fakeBinName, "/fallback/"+fakeBinName)
	if resolved != fakeBinPath {
		t.Errorf("expected %q, got %q", fakeBinPath, resolved)
	}
}

func TestFindCommandOrEmpty(t *testing.T) {
	fakeName := "definitely_nonexistent_tool_99999"
	path := FindCommandOrEmpty(fakeName)
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}
