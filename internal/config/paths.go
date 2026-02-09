package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// GetConfigBasePath returns the base directory for config files
func GetConfigBasePath() string {
	if isAndroid() {
		return "/data/local/tmp/wifi_tools"
	}
	return "." // Current directory on Linux
}

// GetConfigPath generates a timestamped config file path
func GetConfigPath(configType, moduleID string) string {
	basePath := GetConfigBasePath()
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s_%s.conf", configType, moduleID, timestamp)
	return filepath.Join(basePath, filename)
}

// EnsureConfigDirectory creates the config directory if it doesn't exist
func EnsureConfigDirectory() error {
	basePath := GetConfigBasePath()
	return os.MkdirAll(basePath, 0755)
}

// SaveConfig writes config content to a file
func SaveConfig(path, content string) error {
	if err := EnsureConfigDirectory(); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("[INFO] Configuration saved to: %s\n", path)
	return nil
}

// isAndroid detects if running on Android
func isAndroid() bool {
	if runtime.GOOS == "android" {
		return true
	}
	_, err := os.Stat("/system/build.prop")
	return err == nil
}
