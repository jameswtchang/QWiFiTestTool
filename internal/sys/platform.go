package sys

import (
	"fmt"
	"os"
	"path/filepath"

	"qwifitesttool/internal/ui"
)

// PlatformInfo holds platform-specific paths and configurations
type PlatformInfo struct {
	IsAndroid           bool
	IsLinux             bool
	Platform            string
	WpaSupplicantSocket string
	WpaCliPath          string
	WpaSupplicantPath   string
}

// DetectPlatform detects the current platform and returns platform-specific information
func DetectPlatform() *PlatformInfo {
	info := &PlatformInfo{}

	// Detect Android
	if _, err := os.Stat("/system/build.prop"); err == nil {
		info.IsAndroid = true
		info.Platform = "Android"
		info.WpaSupplicantSocket = detectAndroidWpaSocket()
		// Try to find wpa_cli in PATH, fallback to common Android locations
		info.WpaCliPath = FindCommand("wpa_cli", "/system/bin/wpa_cli")
		info.WpaSupplicantPath = FindCommand("wpa_supplicant", "/system/bin/wpa_supplicant")
	} else {
		// Linux
		info.IsLinux = true
		info.Platform = "Linux"
		info.WpaSupplicantSocket = detectLinuxWpaSocket()
		// Try to find wpa_cli in PATH, fallback to common Linux locations
		info.WpaCliPath = FindCommand("wpa_cli", "/usr/sbin/wpa_cli")
		info.WpaSupplicantPath = FindCommand("wpa_supplicant", "/usr/sbin/wpa_supplicant")
	}

	return info
}

// detectAndroidWpaSocket detects wpa_supplicant socket path on Android
func detectAndroidWpaSocket() string {
	// Common Android socket paths (in order of likelihood)
	candidates := []string{
		"/data/vendor/wifi/wpa/sockets",   // Android 10+ (Vendor partition)
		"/data/misc/wifi/sockets",          // Android 8-9
		"/data/system/wpa_supplicant",      // Older Android
		"/var/run/wpa_supplicant",          // Fallback
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Default fallback
	fmt.Println("[WARN] Could not detect Android socket path, using default: /data/misc/wifi/sockets")
	return "/data/misc/wifi/sockets"
}

// detectLinuxWpaSocket detects wpa_supplicant socket path on Linux
func detectLinuxWpaSocket() string {
	// Common Linux socket paths (in order of likelihood)
	candidates := []string{
		"/run/wpa_supplicant",    // Modern Linux (systemd)
		"/var/run/wpa_supplicant", // Traditional Linux
		"/tmp/wpa_supplicant",     // Alternative fallback
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// If none exist, try to create the directory (for fresh installations)
	defaultPath := "/var/run/wpa_supplicant"
	if err := os.MkdirAll(defaultPath, 0755); err == nil {
		fmt.Printf("[INFO] Created socket directory: %s\n", defaultPath)
		return defaultPath
	}

	fmt.Println("[WARN] Could not detect Linux socket path, using default: /var/run/wpa_supplicant")
	return defaultPath
}

// GetWpaSocketForInterface returns the full socket path for a specific interface
func GetWpaSocketForInterface(basePath, iface string) string {
	return filepath.Join(basePath, iface)
}

// Print displays platform information
func (pi *PlatformInfo) Print() {
	fmt.Println("\n" + ui.Cyan + ui.Bold + "========== 平台資訊 ==========" + ui.Reset)
	fmt.Printf(ui.Bold+"平台: "+ui.Green+"%s"+ui.Reset+"\n", pi.Platform)
	fmt.Printf(ui.Bold+"wpa_supplicant Socket: "+ui.Cyan+"%s"+ui.Reset+"\n", pi.WpaSupplicantSocket)
	fmt.Printf(ui.Bold+"wpa_supplicant 路徑: "+ui.Cyan+"%s"+ui.Reset+"\n", pi.WpaSupplicantPath)
	fmt.Printf(ui.Bold+"wpa_cli 路徑: "+ui.Cyan+"%s"+ui.Reset+"\n", pi.WpaCliPath)
	fmt.Println(ui.Cyan + ui.Bold + "==============================" + ui.Reset)
}
