package sys

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"qwifitesttool/internal/models"
)

// Embedded binary assets for Android ARM64
//
//go:embed wpa_supplicant
var wpaSupplicantBinary []byte

//go:embed hostapd
var hostapdBinary []byte

//go:embed iw
var iwBinary []byte

const (
	androidExtractPath = "/data/local/tmp/wifi_tools"
	wpaSupplicantName  = "wpa_supplicant"
	hostapdName        = "hostapd"
	iwName             = "iw"
)

// elfMagic is the 4-byte header of every ELF executable (0x7F 'E' 'L' 'F')
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// BinaryManager handles extraction and path resolution of embedded binaries
type BinaryManager struct {
	platform      string
	extractPath   string
	binariesReady bool
	extracted     map[string]bool // binaries actually extracted as valid ELF executables
}

// NewBinaryManager creates a new binary manager instance
func NewBinaryManager() *BinaryManager {
	platform := runtime.GOOS
	if isAndroid() {
		platform = "android"
	}

	return &BinaryManager{
		platform:      platform,
		extractPath:   getExtractPath(platform),
		binariesReady: false,
		extracted:     make(map[string]bool),
	}
}

// Initialize extracts embedded binaries if on Android
func (bm *BinaryManager) Initialize() error {
	if bm.platform != "android" {
		// On Linux, use system binaries
		bm.binariesReady = true
		return nil
	}

	// Create extraction directory
	if err := os.MkdirAll(bm.extractPath, 0755); err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}

	// Extract binaries
	binaries := map[string][]byte{
		wpaSupplicantName: wpaSupplicantBinary,
		hostapdName:       hostapdBinary,
		iwName:            iwBinary,
	}

	for name, data := range binaries {
		targetPath := filepath.Join(bm.extractPath, name)

		// Embedded assets may still be text placeholders; extracting them would yield
		// an "executable" that fails with exec format error. Fall back to system binaries.
		if !isELF(data) {
			fmt.Printf("[WARN] Embedded %s is not a valid ELF binary (placeholder), using system binary instead\n", name)
			removeStalePlaceholder(targetPath)
			continue
		}

		if err := extractBinary(targetPath, data); err != nil {
			return fmt.Errorf("failed to extract %s: %w", name, err)
		}
		bm.extracted[name] = true
		fmt.Printf("[INFO] Extracted %s to %s\n", name, targetPath)
	}

	bm.binariesReady = true
	return nil
}

// GetWpaSupplicantPath returns the absolute path to wpa_supplicant
func (bm *BinaryManager) GetWpaSupplicantPath() string {
	if bm.extracted[wpaSupplicantName] {
		return filepath.Join(bm.extractPath, wpaSupplicantName)
	}
	if bm.platform == "android" {
		return FindCommand("wpa_supplicant", "/vendor/bin/hw/wpa_supplicant")
	}
	// Try to find in PATH first, then use common paths
	return FindCommand("wpa_supplicant", "/usr/sbin/wpa_supplicant")
}

// GetWpaSupplicantPathFor returns the appropriate wpa_supplicant binary path for a given module
func (bm *BinaryManager) GetWpaSupplicantPathFor(module *models.WiFiModule) string {
	if module != nil && module.ID == "quectel_fgh200m" {
		return FindCommand("wpa_supplicant_s1g", "/usr/sbin/wpa_supplicant_s1g")
	}
	return bm.GetWpaSupplicantPath()
}

// GetHostapdPath returns the absolute path to hostapd
func (bm *BinaryManager) GetHostapdPath() string {
	if bm.extracted[hostapdName] {
		return filepath.Join(bm.extractPath, hostapdName)
	}
	if bm.platform == "android" {
		return FindCommand("hostapd", "/vendor/bin/hw/hostapd")
	}
	return FindCommand("hostapd", "/usr/sbin/hostapd")
}

// GetHostapdPathFor returns the appropriate hostapd binary path for a given module
func (bm *BinaryManager) GetHostapdPathFor(module *models.WiFiModule) string {
	if module != nil && module.ID == "quectel_fgh200m" {
		return FindCommand("hostapd_s1g", "/usr/sbin/hostapd_s1g")
	}
	return bm.GetHostapdPath()
}

// GetWpaCliPath returns the absolute path to wpa_cli
func (bm *BinaryManager) GetWpaCliPath() string {
	if bm.platform == "android" {
		return FindCommand("wpa_cli", "/system/bin/wpa_cli")
	}
	return FindCommand("wpa_cli", "/usr/sbin/wpa_cli")
}

// GetIwPath returns the absolute path to iw tool
func (bm *BinaryManager) GetIwPath() string {
	if bm.extracted[iwName] {
		return filepath.Join(bm.extractPath, iwName)
	}
	return FindCommand("iw", "/usr/sbin/iw")
}

// IsAndroid returns true if running on Android
func (bm *BinaryManager) IsAndroid() bool {
	return bm.platform == "android"
}

// extractBinary writes binary data to a file and sets executable permissions
func extractBinary(targetPath string, data []byte) error {
	file, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(file, io.NopCloser(io.Reader(bytes.NewReader(data)))); err != nil {
		return err
	}

	// Set executable permissions
	return os.Chmod(targetPath, 0755)
}

// isELF reports whether data starts with the ELF magic header
func isELF(data []byte) bool {
	return bytes.HasPrefix(data, elfMagic)
}

// removeStalePlaceholder deletes a non-ELF file left at path by older tool versions,
// so FindCommand never resolves to a previously extracted placeholder.
func removeStalePlaceholder(path string) {
	head := make([]byte, len(elfMagic))
	f, err := os.Open(path)
	if err != nil {
		return
	}
	n, _ := io.ReadFull(f, head)
	f.Close()
	if !isELF(head[:n]) {
		_ = os.Remove(path)
	}
}

// getExtractPath returns the platform-specific extraction path
func getExtractPath(platform string) string {
	if platform == "android" {
		return androidExtractPath
	}
	return "" // Not used on Linux
}

// isAndroid detects if running on Android
func isAndroid() bool {
	_, err := os.Stat("/system/build.prop")
	return err == nil
}
