package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckWpaBinarySupport(t *testing.T) {
	// 1. Missing file should default to true (to not block non-Linux/dev environments)
	if !CheckWpaBinarySupport("/nonexistent/wpa_supplicant", "disable_ht40") {
		t.Errorf("Expected nonexistent binary to default to true")
	}

	// 2. Mock support test
	MockWpaCaps = map[string]bool{
		"disable_ht40": false,
		"disable_eht":  true,
	}
	defer func() { MockWpaCaps = nil }()

	if CheckWpaBinarySupport("/any/path", "disable_ht40") {
		t.Errorf("Expected MockWpaCaps to report false for disable_ht40")
	}
	if !CheckWpaBinarySupport("/any/path", "disable_eht") {
		t.Errorf("Expected MockWpaCaps to report true for disable_eht")
	}

	// 3. Real file inspection test
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "dummy_wpa_supplicant")
	dummyContent := []byte("...padding...disable_eht\x00...more_data...")
	if err := os.WriteFile(binPath, dummyContent, 0755); err != nil {
		t.Fatalf("Failed to write dummy binary: %v", err)
	}

	MockWpaCaps = nil
	if !CheckWpaBinarySupport(binPath, "disable_eht") {
		t.Errorf("Expected dummy binary to support disable_eht")
	}
	if CheckWpaBinarySupport(binPath, "disable_ht40") {
		t.Errorf("Expected dummy binary to NOT support disable_ht40")
	}

	// 4. Test dynamic param outside commonParams
	if CheckWpaBinarySupport(binPath, "disable_custom_unlisted") {
		t.Errorf("Expected dummy binary to NOT support disable_custom_unlisted")
	}
}
