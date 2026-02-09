package sys

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// BinaryManager handles extraction and path resolution of embedded binaries
type BinaryManager struct {
	platform      string
	extractPath   string
	binariesReady bool
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
		if len(data) == 0 {
			fmt.Printf("[WARN] Binary %s is empty, skipping extraction\n", name)
			continue
		}

		targetPath := filepath.Join(bm.extractPath, name)
		if err := extractBinary(targetPath, data); err != nil {
			return fmt.Errorf("failed to extract %s: %w", name, err)
		}
		fmt.Printf("[INFO] Extracted %s to %s\n", name, targetPath)
	}

	bm.binariesReady = true
	return nil
}

// GetWpaSupplicantPath returns the absolute path to wpa_supplicant
func (bm *BinaryManager) GetWpaSupplicantPath() string {
	if bm.platform == "android" {
		return filepath.Join(bm.extractPath, wpaSupplicantName)
	}
	// Try to find in PATH first, then use common paths
	return FindCommand("wpa_supplicant", "/usr/sbin/wpa_supplicant")
}

// GetHostapdPath returns the absolute path to hostapd
func (bm *BinaryManager) GetHostapdPath() string {
	if bm.platform == "android" {
		return filepath.Join(bm.extractPath, hostapdName)
	}
	return FindCommand("hostapd", "/usr/sbin/hostapd")
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
	if bm.platform == "android" {
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
