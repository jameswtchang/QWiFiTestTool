package config

import (
	"fmt"
	"os"
	"path/filepath"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
)

// ConfigManager handles configuration file loading with three-tier priority:
// 1. Custom config (user-saved, highest priority)
// 2. System default config
// 3. Auto-generated config (lowest priority)
type ConfigManager struct {
	configType string // "wpa_supplicant", "hostapd", or "dnsmasq"
	module     *models.WiFiModule
}

// NewConfigManager creates a new config manager
func NewConfigManager(configType string, module *models.WiFiModule) *ConfigManager {
	return &ConfigManager{
		configType: configType,
		module:     module,
	}
}

// GetConfigPath returns the config path with three-tier priority
func (cm *ConfigManager) GetConfigPath() (string, bool, error) {
	// Tier 1: Custom config (user-saved)
	customPath := cm.getCustomConfigPath()
	if fileExists(customPath) {
		fmt.Println(i18n.Tf("cfg_mgr.custom", customPath))
		return customPath, true, nil
	}

	// Tier 2: System default config
	systemPaths := cm.getSystemConfigPaths()
	for _, path := range systemPaths {
		if fileExists(path) {
			fmt.Println(i18n.Tf("cfg_mgr.system", path))
			return path, true, nil
		}
	}

	// Tier 3: Auto-generate config
	generatedPath := cm.getGeneratedConfigPath()
	fmt.Println(i18n.Tf("cfg_mgr.generate", generatedPath))
	return generatedPath, false, nil
}

// GetConfigPathSilent returns the config path without logging to stdout
func (cm *ConfigManager) GetConfigPathSilent() (string, bool, error) {
	customPath := cm.getCustomConfigPath()
	if fileExists(customPath) {
		return customPath, true, nil
	}

	systemPaths := cm.getSystemConfigPaths()
	for _, path := range systemPaths {
		if fileExists(path) {
			return path, true, nil
		}
	}

	generatedPath := cm.getGeneratedConfigPath()
	return generatedPath, false, nil
}

// SaveAsCustomConfig saves current config as custom config for future use
func (cm *ConfigManager) SaveAsCustomConfig(content string) error {
	customPath := cm.getCustomConfigPath()

	if err := EnsureConfigDirectory(); err != nil {
		return fmt.Errorf(i18n.T("cfg_mgr.mkdir_fail"), err)
	}

	if err := os.WriteFile(customPath, []byte(content), 0644); err != nil {
		return fmt.Errorf(i18n.T("cfg_mgr.save_fail"), err)
	}

	fmt.Println(i18n.Tf("cfg_mgr.saved", customPath))
	return nil
}

// getCustomConfigPath returns the path for custom (user-saved) config
func (cm *ConfigManager) getCustomConfigPath() string {
	basePath := GetConfigBasePath()
	filename := fmt.Sprintf("%s_%s_custom.conf", cm.configType, cm.module.ID)
	return filepath.Join(basePath, filename)
}

// getSystemConfigPaths returns possible system default config paths
func (cm *ConfigManager) getSystemConfigPaths() []string {
	switch cm.configType {
	case "wpa_supplicant":
		return []string{
			"/etc/wpa_supplicant.conf",
			"/etc/wpa_supplicant/wpa_supplicant.conf",
		}
	case "hostapd":
		return []string{
			"/etc/hostapd.conf",
			"/etc/hostapd/hostapd.conf",
		}
	case "dnsmasq":
		return []string{
			"/etc/dnsmasq.conf",
		}
	default:
		return []string{}
	}
}

// getGeneratedConfigPath returns the path for auto-generated config
func (cm *ConfigManager) getGeneratedConfigPath() string {
	// Use simpler naming without timestamp for consistency
	basePath := GetConfigBasePath()
	filename := fmt.Sprintf("%s_%s.conf", cm.configType, cm.module.ID)
	return filepath.Join(basePath, filename)
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// PromptSaveCustom asks user if they want to save as custom config
func (cm *ConfigManager) PromptSaveCustom() bool {
	fmt.Print(i18n.T("cfg_mgr.prompt_save"))
	var response string
	fmt.Scanln(&response)
	return response == "y" || response == "Y"
}
