package wifi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qwifitesttool/internal/sys"
)

func TestUpdateWpaSupplicantConfig_PreservesPolicies(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "wpa_supplicant.conf")

	initialContent := `ctrl_interface=/var/run/wpa_supplicant
update_config=1

network={
	ssid="DummyAP_For_Test"
	freq_list=5180 5200 5240
	disable_eht=1
	disable_he=1
	disable_ht40=1
}
`
	if err := os.WriteFile(confPath, []byte(initialContent), 0600); err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}

	err := updateWpaSupplicantConfig(confPath, "MyNewAP", "12345678", "[WPA2-PSK-CCMP][ESS]")
	if err != nil {
		t.Fatalf("updateWpaSupplicantConfig failed: %v", err)
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("Failed to read back file: %v", err)
	}

	res := string(data)

	if !strings.Contains(res, `ssid="MyNewAP"`) {
		t.Errorf("Expected new SSID 'MyNewAP', got:\n%s", res)
	}
	if !strings.Contains(res, `psk="12345678"`) {
		t.Errorf("Expected psk, got:\n%s", res)
	}
	if !strings.Contains(res, `freq_list=5180 5200 5240`) {
		t.Errorf("Expected freq_list to be preserved, got:\n%s", res)
	}
	if !strings.Contains(res, `disable_eht=1`) {
		t.Errorf("Expected disable_eht=1 to be preserved, got:\n%s", res)
	}
	if !strings.Contains(res, `disable_he=1`) {
		t.Errorf("Expected disable_he=1 to be preserved, got:\n%s", res)
	}
	if !strings.Contains(res, `disable_ht40=1`) {
		t.Errorf("Expected disable_ht40=1 to be preserved, got:\n%s", res)
	}
	if strings.Contains(res, "DummyAP_For_Test") {
		t.Errorf("Old SSID DummyAP_For_Test should not exist, got:\n%s", res)
	}
}

func TestConnector_SanitizeConfigForDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "wpa_supplicant.conf")

	content := `ctrl_interface=/var/run/wpa_supplicant
update_config=1

network={
	ssid="TestAP"
	psk="secret123"
	# disable_ht40=0 (commented out, should not be stripped or trigger warning)
	disable_ht40 = 1
	disable_he=1
}
`
	if err := os.WriteFile(confPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}

	// Mock wpa caps: disable_ht40 is unsupported, disable_he is supported
	sys.MockWpaCaps = map[string]bool{
		"disable_ht40": false,
		"disable_he":   true,
	}
	defer func() { sys.MockWpaCaps = nil }()

	conn := &Connector{
		wpaSupplicant: "mock_wpa_supplicant",
		configPath:    confPath,
	}

	sanitizedPath, stripped, err := conn.sanitizeConfigForDaemon(confPath)
	if err != nil {
		t.Fatalf("sanitizeConfigForDaemon failed: %v", err)
	}

	if len(stripped) != 1 || stripped[0] != "disable_ht40" {
		t.Fatalf("Expected stripped to be [disable_ht40], got: %v", stripped)
	}

	sanitizedData, err := os.ReadFile(sanitizedPath)
	if err != nil {
		t.Fatalf("Failed to read sanitized file: %v", err)
	}
	sanitizedStr := string(sanitizedData)

	if strings.Contains(sanitizedStr, "disable_ht40 = 1") || strings.Contains(sanitizedStr, "disable_ht40=1") {
		t.Errorf("Expected disable_ht40=1 to be stripped, got:\n%s", sanitizedStr)
	}
	if !strings.Contains(sanitizedStr, "disable_he=1") {
		t.Errorf("Expected disable_he=1 to be preserved, got:\n%s", sanitizedStr)
	}
	if !strings.Contains(sanitizedStr, "# disable_ht40=0") {
		t.Errorf("Expected comment to be preserved, got:\n%s", sanitizedStr)
	}

	// Verify original file was left untouched
	originalData, _ := os.ReadFile(confPath)
	if !strings.Contains(string(originalData), "disable_ht40 = 1") {
		t.Errorf("Original file should have been untouched")
	}
}
