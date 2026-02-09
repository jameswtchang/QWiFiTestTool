package models

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"os"
	"runtime"
)

//go:embed default_modules.xml
var defaultModulesXML []byte

// LoadModules loads WiFi module configurations using a hybrid strategy:
// 1. Try external file (./default_modules.xml on Linux, /data/local/tmp/default_modules.xml on Android)
// 2. Fall back to embedded default configuration
func LoadModules() (*WiFiToolConfig, error) {
	// Determine external path based on platform
	externalPath := getExternalConfigPath()

	// Try loading from external file first
	if data, err := os.ReadFile(externalPath); err == nil {
		fmt.Printf("[INFO] 使用外部配置檔案: %s\n", externalPath)
		return parseXML(data)
	}

	// Fall back to embedded configuration
	fmt.Println("[INFO] 未找到外部配置檔案，使用內嵌預設配置")
	return parseXML(defaultModulesXML)
}

// getExternalConfigPath returns the platform-specific external config path
func getExternalConfigPath() string {
	if runtime.GOOS == "android" || isAndroid() {
		return "/data/local/tmp/default_modules.xml"
	}
	return "./default_modules.xml"
}

// isAndroid attempts to detect if running on Android by checking for Android-specific paths
func isAndroid() bool {
	_, err := os.Stat("/system/build.prop")
	return err == nil
}

// parseXML parses XML data into WiFiToolConfig struct
func parseXML(data []byte) (*WiFiToolConfig, error) {
	var config WiFiToolConfig
	if err := xml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	if len(config.Modules) == 0 {
		return nil, fmt.Errorf("no modules found in configuration")
	}

	return &config, nil
}

// GetModuleByID searches for a module by its ID
func (c *WiFiToolConfig) GetModuleByID(id string) (*WiFiModule, error) {
	for i := range c.Modules {
		if c.Modules[i].ID == id {
			return &c.Modules[i], nil
		}
	}
	return nil, fmt.Errorf("module with ID '%s' not found", id)
}

// ListModules returns a formatted list of available modules
func (c *WiFiToolConfig) ListModules() []string {
	result := make([]string, len(c.Modules))
	for i, mod := range c.Modules {
		result[i] = fmt.Sprintf("[%d] %s (%s)", i+1, mod.DisplayName, mod.ID)
	}
	return result
}
