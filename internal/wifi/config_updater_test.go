package wifi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if strings.Contains(res, "DummyAP_For_Test") {
		t.Errorf("Old SSID DummyAP_For_Test should not exist, got:\n%s", res)
	}
}
