package sys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsELF(t *testing.T) {
	if !isELF([]byte{0x7f, 'E', 'L', 'F', 0x02}) {
		t.Error("expected ELF header to be detected")
	}
	if isELF([]byte("# Placeholder for hostapd binary")) {
		t.Error("expected text placeholder to be rejected")
	}
	if isELF(nil) {
		t.Error("expected empty data to be rejected")
	}
}

// The repository ships text placeholders for the Android binaries; Initialize must not
// extract them, and path getters must fall back to system binaries instead.
func TestInitialize_SkipsPlaceholderBinaries(t *testing.T) {
	tmpDir := t.TempDir()

	// Simulate a placeholder extracted by an older tool version
	stale := filepath.Join(tmpDir, hostapdName)
	if err := os.WriteFile(stale, []byte("# Placeholder\n"), 0755); err != nil {
		t.Fatal(err)
	}

	bm := &BinaryManager{platform: "android", extractPath: tmpDir, extracted: make(map[string]bool)}
	if err := bm.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	for _, name := range []string{wpaSupplicantName, hostapdName, iwName} {
		if bm.extracted[name] {
			t.Errorf("placeholder %s must not be marked as extracted", name)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale placeholder should have been removed")
	}

	for name, path := range map[string]string{
		"wpa_supplicant": bm.GetWpaSupplicantPath(),
		"hostapd":        bm.GetHostapdPath(),
		"iw":             bm.GetIwPath(),
	} {
		if strings.HasPrefix(path, tmpDir) {
			t.Errorf("%s resolved to extraction dir %q, expected system fallback", name, path)
		}
	}
}

func TestRemoveStalePlaceholder_KeepsELF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostapd")
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}, 0755); err != nil {
		t.Fatal(err)
	}
	removeStalePlaceholder(path)
	if _, err := os.Stat(path); err != nil {
		t.Error("valid ELF binary must not be removed")
	}
}

func TestExtractedBinaryPreferred(t *testing.T) {
	tmpDir := t.TempDir()
	bm := &BinaryManager{platform: "android", extractPath: tmpDir, extracted: map[string]bool{hostapdName: true}}
	if got, want := bm.GetHostapdPath(), filepath.Join(tmpDir, hostapdName); got != want {
		t.Errorf("expected extracted path %q, got %q", want, got)
	}
}

func TestServiceManager_NilAndLinuxNoop(t *testing.T) {
	var nilSM *ServiceManager
	if err := nilSM.DisableAndroidWiFi(); err != nil {
		t.Errorf("nil Disable: %v", err)
	}
	if err := nilSM.EnableAndroidWiFi(); err != nil {
		t.Errorf("nil Enable: %v", err)
	}
	if nilSM.IsDisabled() {
		t.Error("nil manager must report not disabled")
	}

	sm := NewServiceManager(false)
	if err := sm.DisableAndroidWiFi(); err != nil || sm.IsDisabled() {
		t.Error("Linux manager must be a no-op")
	}
}
