package models

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"qwifitesttool/assets"
	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/platform"
)

// LoadModules loads WiFi module configurations using a hybrid strategy:
// 1. Try external file (./default_modules.xml on Linux, /data/local/tmp/default_modules.xml on Android)
// 2. Fall back to embedded default configuration (from assets package)
func LoadModules() (*WiFiToolConfig, error) {
	// Determine external path based on platform
	externalPath := getExternalConfigPath()

	// Try loading from external file first
	if data, err := os.ReadFile(externalPath); err == nil {
		fmt.Println(i18n.Tf("loader.external", externalPath))
		if !bytes.Equal(data, assets.DefaultModulesXML) {
			fmt.Println(i18n.T("loader.custom"))
		}
		return parseXML(data)
	}

	// Fall back to embedded configuration
	fmt.Println(i18n.T("loader.embedded"))
	return parseXML(assets.DefaultModulesXML)
}

// getExternalConfigPath returns the platform-specific external config path
func getExternalConfigPath() string {
	if platform.IsAndroid() {
		return "/data/local/tmp/default_modules.xml"
	}
	exePath, err := os.Executable()
	if err == nil {
		fromExe := filepath.Join(filepath.Dir(exePath), "default_modules.xml")
		if _, err := os.Stat(fromExe); err == nil {
			return fromExe
		}
	}
	return "./default_modules.xml"
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
		parts := strings.SplitN(mod.DisplayName, " (", 2)
		if len(parts) == 2 {
			result[i] = fmt.Sprintf("[%d] %-22s (%s", i+1, parts[0], parts[1])
		} else {
			result[i] = fmt.Sprintf("[%d] %s", i+1, mod.DisplayName)
		}
	}
	return result
}
