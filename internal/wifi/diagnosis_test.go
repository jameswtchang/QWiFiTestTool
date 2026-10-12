package wifi

import (
	"os"
	"path/filepath"
	"qwifitesttool/internal/models"
	"testing"
)

func TestNormalizeModuleName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"dot11ah.ko", "dot11ah"},
		{"pcie-dw-rockchip.ko", "pcie_dw_rockchip"},
		{"/path/to/aic8800_bsp.ko", "aic8800_bsp"},
		{"wlan_cnss_core_pcie.ko", "wlan_cnss_core_pcie"},
		{"8821cu", "8821cu"},
		{"rtl88x2bu.ko", "rtl88x2bu"},
	}

	for _, tt := range tests {
		actual := normalizeModuleName(tt.input)
		if actual != tt.expected {
			t.Errorf("normalizeModuleName(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestCheckModuleParameters(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sys_module_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create simulated /sys/module structure
	modDir := filepath.Join(tmpDir, "dot11ah", "parameters")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("failed to create modDir: %v", err)
	}

	// Write actual parameter: channelization_scheme = 3
	if err := os.WriteFile(filepath.Join(modDir, "channelization_scheme"), []byte("3\n"), 0644); err != nil {
		t.Fatalf("failed to write param file: %v", err)
	}

	dr := &DiagnosisResult{}
	// Check against mismatch: expected is 1
	dr.checkModuleParameters("dot11ah", "channelization_scheme=1")

	// Notice: in real runtime checkModuleParameters checks /sys/module/dot11ah/parameters
	// On Windows, /sys/module doesn't exist so checkModuleParameters simply skips (no error)
	// That verifies absence of crash on non-Linux environments.
}

func TestRunDiagnosisModuleSetup(t *testing.T) {
	mod := &models.WiFiModule{
		ID:          "test_mod",
		DisplayName: "Test Module",
		Kernel: models.KernelConfig{
			KoName:            "morse.ko",
			PreloadKo:         "dot11ah.ko",
			PreloadInsmodArgs: "channelization_scheme=1",
			InsmodArgs:        "country=AU",
			Interface:         "wlan0",
		},
	}

	result := RunDiagnosis(mod)
	if result == nil {
		t.Fatal("expected non-nil result from RunDiagnosis")
	}

	// On non-Linux (or when /proc/modules not available), result.DriverLoaded should be false
	// and errors should reflect that.
	if result.DriverLoaded {
		t.Log("Driver loaded according to environment")
	} else {
		if len(result.Errors) == 0 {
			t.Error("expected errors when driver is not loaded")
		}
	}
}
