package config

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
)

// WpaEditor handles interactive wpa_supplicant.conf editing
type WpaEditor struct {
	module       *models.WiFiModule
	platformInfo *sys.PlatformInfo
	configPath   string
	reader       *bufio.Reader
}

// NewWpaEditor creates a new WPA configuration editor
func NewWpaEditor(module *models.WiFiModule, platformInfo *sys.PlatformInfo) *WpaEditor {
	return &WpaEditor{
		module:       module,
		platformInfo: platformInfo,
		reader:       bufio.NewReader(os.Stdin),
	}
}

// Start begins the interactive editing session
// Returns (actionTaken, error) - actionTaken is false if user skipped editing
func (e *WpaEditor) Start() (bool, error) {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("wpa_ed.title") + " ==========" + ui.Reset)

	// Step 1: Detect config path
	e.configPath = e.detectConfigPath()
	fmt.Printf(ui.Cyan+"[INFO] "+ui.Reset+i18n.Tf("wpa_ed.config_path", e.configPath)+"\n")

	// Step 2: Load or create config
	content, err := e.loadOrCreateConfig()
	if err != nil {
		return false, fmt.Errorf(i18n.T("wpa_ed.load_fail"), err)
	}

	// Step 3: Display current config
	e.displayConfig(content)

	// Step 4: Ask user for action
	fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.T("wpa_ed.action_title") + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "[1]" + ui.Reset + " " + i18n.T("wpa_ed.action.1"))
	fmt.Println(ui.Cyan + ui.Bold + "[2]" + ui.Reset + " " + i18n.T("wpa_ed.action.2"))
	fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " " + i18n.T("wpa_ed.action.0"))
	fmt.Print(ui.Yellow + ui.Bold + i18n.T("wpa_ed.action.select") + ui.Reset)

	input, _ := e.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var newContent string

	if input == "1" {
		// Step 5: Interactive edit
		var err error
		newContent, err = e.interactiveEdit(content)
		if err != nil {
			return false, fmt.Errorf(i18n.T("wpa_ed.edit_fail"), err)
		}
	} else if input == "2" {
		newContent = e.generateDefaultConfig()
		fmt.Println(ui.Cyan + "[INFO] " + ui.Reset + i18n.T("wpa_ed.xml_loaded"))
	} else {
		fmt.Println(ui.Cyan + "[INFO] " + ui.Reset + i18n.T("wpa_ed.no_change"))
		return false, nil
	}

	// Step 6: Show final config and ask to apply
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("wpa_ed.edited_title") + " ==========" + ui.Reset)
	fmt.Println(ui.Green + newContent + ui.Reset)
	fmt.Println(ui.Blue + ui.Bold + "======================================" + ui.Reset)

	fmt.Print("\n" + ui.Yellow + ui.Bold + i18n.T("wpa_ed.apply_confirm") + ui.Reset)
	response, _ := e.reader.ReadString('\n')
	response = strings.TrimSpace(response)

	if response != "y" && response != "Y" {
		fmt.Println(ui.Cyan + "[INFO] " + ui.Reset + i18n.T("wpa_ed.apply_cancelled"))
		return false, nil
	}

	// Step 7: Save config
	if err := e.saveConfig(newContent); err != nil {
		return false, fmt.Errorf(i18n.T("wpa_ed.save_fail"), err)
	}

	fmt.Println("\n" + i18n.T("wpa_ed.save_ok"))

	// Step 8: Read back and display the saved config
	fmt.Println("\n========== " + i18n.T("wpa_ed.saved_title") + " ==========")
	if savedData, err := os.ReadFile(e.configPath); err == nil {
		fmt.Println(string(savedData))
	} else {
		fmt.Printf(i18n.Tf("wpa_ed.read_saved_fail", err)+"\n")
	}
	fmt.Println("==================================")

	// Step 9: Restart network if needed
	if err := e.restartNetworkIfNeeded(); err != nil {
		fmt.Printf(i18n.Tf("wpa_ed.net_restart_fail", err)+"\n")
	}

	return true, nil
}

// detectConfigPath detects the system wpa_supplicant.conf path
func (e *WpaEditor) detectConfigPath() string {
	var candidates []string

	if e.platformInfo.IsAndroid {
		candidates = []string{
			"/data/misc/wifi/wpa_supplicant.conf",
			"/data/system/wpa_supplicant/wpa_supplicant.conf",
			"/data/local/tmp/wifi_tools/wpa_supplicant.conf",
		}
	} else {
		candidates = []string{
			"/etc/wpa_supplicant.conf",
			"/etc/wpa_supplicant/wpa_supplicant.conf",
		}
	}

	// Check which path exists
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Return first candidate as default
	return candidates[0]
}

// loadOrCreateConfig loads existing config or creates a default one
func (e *WpaEditor) loadOrCreateConfig() (string, error) {
	// Try to load existing file
	data, err := os.ReadFile(e.configPath)
	if err == nil {
		fmt.Println(i18n.T("wpa_ed.loaded_existing"))
		return string(data), nil
	}

	// File doesn't exist, create default from XML
	fmt.Println(i18n.T("wpa_ed.not_exist"))

	defaultConfig := e.generateDefaultConfig()
	return defaultConfig, nil
}

// generateDefaultConfig creates a default config based on XML module settings
func (e *WpaEditor) generateDefaultConfig() string {
	var sb strings.Builder

	sb.WriteString("# wpa_supplicant configuration\n")
	sb.WriteString("# Auto-generated by QWifiTestTool\n\n")

	// Collect global and network params
	globalParams := make(map[string]string)
	networkParams := make(map[string]string)

	for _, param := range e.module.StationConfig {
		if strings.HasPrefix(param.Key, "network_") {
			networkParams[strings.TrimPrefix(param.Key, "network_")] = param.Value
		} else {
			globalParams[param.Key] = param.Value
		}
	}

	// Write global params
	if ctrlInterface, ok := globalParams["ctrl_interface"]; ok {
		sb.WriteString(fmt.Sprintf("ctrl_interface=%s\n", ctrlInterface))
		delete(globalParams, "ctrl_interface") // avoid double write
	} else {
		sb.WriteString(fmt.Sprintf("ctrl_interface=%s\n", e.platformInfo.WpaSupplicantSocket))
	}

	// Write remaining global params (only what's defined in XML)
	for k, v := range globalParams {
		sb.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}

	sb.WriteString("\n")

	// Write network block if we have network params
	if len(networkParams) > 0 {
		sb.WriteString("network={\n")
		// Output ssid with quotes if not already quoted
		if ssid, ok := networkParams["ssid"]; ok {
			if !strings.HasPrefix(ssid, "\"") {
				ssid = "\"" + ssid + "\""
			}
			sb.WriteString(fmt.Sprintf("\tssid=%s\n", ssid))
			delete(networkParams, "ssid")
		}
		// Output psk with quotes if not already quoted
		if psk, ok := networkParams["psk"]; ok {
			if !strings.HasPrefix(psk, "\"") {
				psk = "\"" + psk + "\""
			}
			sb.WriteString(fmt.Sprintf("\tpsk=%s\n", psk))
			delete(networkParams, "psk")
		}
		// Output sae_password with quotes if not already quoted
		if saePw, ok := networkParams["sae_password"]; ok {
			if !strings.HasPrefix(saePw, "\"") {
				saePw = "\"" + saePw + "\""
			}
			sb.WriteString(fmt.Sprintf("\tsae_password=%s\n", saePw))
			delete(networkParams, "sae_password")
		}
		// Output remaining network params
		for k, v := range networkParams {
			sb.WriteString(fmt.Sprintf("\t%s=%s\n", k, v))
		}
		sb.WriteString("}\n")
	} else {
		sb.WriteString("# Network configurations will be added here\n")
		sb.WriteString("# Use wpa_cli or Station mode to add networks\n")
	}

	return sb.String()
}

// getParamFromXML retrieves a parameter from the module's StationConfig
func (e *WpaEditor) getParamFromXML(key string) string {
	for _, param := range e.module.StationConfig {
		if param.Key == key {
			return param.Value
		}
	}
	return ""
}

// displayConfig shows the current configuration
func (e *WpaEditor) displayConfig(content string) {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("wpa_ed.current_title") + " ==========" + ui.Reset)
	fmt.Println(ui.Green + content + ui.Reset)
	fmt.Println(ui.Blue + ui.Bold + "==================================" + ui.Reset)
}

// interactiveEdit performs menu-based parameter editing
func (e *WpaEditor) interactiveEdit(content string) (string, error) {
	params := GetWpaParams()
	configMap := parseConfig(content)
	reader := e.reader

	for {
		// Display current config state
		fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("wpa_ed.params_title") + " ==========" + ui.Reset)
		e.displayConfigMap(configMap)
		fmt.Println(ui.Blue + ui.Bold + "==================================" + ui.Reset)

		// Display parameter menu
		fmt.Println("\n" + ui.Yellow + ui.Bold + "========== " + i18n.T("wpa_ed.editable_title") + " ==========" + ui.Reset)
		for i, param := range params {
			if currentVal, exists := configMap[param.Key]; exists {
				fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %-20s ("+i18n.T("wpa_ed.current_val")+ui.Green+"%s"+ui.Reset+")\n", i+1, param.Key, currentVal)
			} else {
				fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %-20s ("+ui.Yellow+i18n.T("wpa_ed.not_set")+ui.Reset+")\n", i+1, param.Key)
			}
		}
		fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " " + i18n.T("wpa_ed.done_edit"))
		fmt.Println(ui.Yellow + ui.Bold + "================================" + ui.Reset)
		fmt.Print(ui.Yellow + ui.Bold + i18n.T("wpa_ed.select_param") + ui.Reset)

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "0" || input == "" {
			break
		}

		var idx int
		fmt.Sscanf(input, "%d", &idx)

		if idx < 1 || idx > len(params) {
			fmt.Println(i18n.T("wpa_ed.invalid_sel"))
			continue
		}

		param := params[idx-1]

		// Show parameter details
		fmt.Printf("\n=== %s ===\n", param.Key)
		fmt.Printf(i18n.T("wpa.param.desc_label")+"%s\n", param.Description())
		fmt.Printf(i18n.T("wpa.param.suggest")+"%v\n", param.SuggestedValues())
		fmt.Printf(i18n.T("wpa.param.default")+"%s\n", param.DefaultValue)
		fmt.Printf(i18n.T("wpa.param.example")+"%s\n", param.Example)

		if current, exists := configMap[param.Key]; exists {
			fmt.Printf(i18n.Tf("wpa_ed.current_val2", current)+"\n")
		}

		fmt.Printf("\n"+i18n.Tf("wpa_ed.input_val", param.DefaultValue))
		value, _ := reader.ReadString('\n')
		value = strings.TrimSpace(value)

		if value == "d" || value == "D" {
			delete(configMap, param.Key)
			fmt.Printf(i18n.Tf("wpa_ed.removed", param.Key)+"\n")
		} else {
			if value == "" {
				value = param.DefaultValue
			}
			configMap[param.Key] = value
			fmt.Printf(i18n.Tf("wpa_ed.set", param.Key, value)+"\n")
		}
	}

	newContent := rebuildConfig(content, configMap)
	return newContent, nil
}

// displayConfigMap shows current config key-value pairs in order
func (e *WpaEditor) displayConfigMap(configMap map[string]string) {
	if len(configMap) == 0 {
		fmt.Println("  " + i18n.T("wpa_ed.empty"))
		return
	}

	// Show known params first in order
	params := GetWpaParams()
	shown := make(map[string]bool)

	for _, param := range params {
		if val, exists := configMap[param.Key]; exists {
			fmt.Printf("  %s=%s\n", param.Key, val)
			shown[param.Key] = true
		}
	}

	// Show remaining params (ctrl_interface, etc.)
	for key, val := range configMap {
		if !shown[key] {
			fmt.Printf("  %s=%s\n", key, val)
		}
	}
}

// parseConfig parses config content into a map
func parseConfig(content string) map[string]string {
	configMap := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			configMap[parts[0]] = parts[1]
		}
	}

	return configMap
}

// rebuildConfig rebuilds the config file with updated parameters
func rebuildConfig(originalContent string, configMap map[string]string) string {
	var sb strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(originalContent))
	updatedKeys := make(map[string]bool)

	// First pass: update existing lines
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip old "# Additional parameters" markers to prevent accumulation
		if trimmed == "# Additional parameters" {
			continue
		}

		// Keep other comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			sb.WriteString(line + "\n")
			continue
		}

		// Check if this line is a key=value pair
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			// Skip if this key was already written (duplicate in original file)
			if updatedKeys[key] {
				continue
			}
			if newValue, exists := configMap[key]; exists {
				sb.WriteString(fmt.Sprintf("%s=%s\n", key, newValue))
				updatedKeys[key] = true
			}
			// Key not in configMap → deleted, skip this line
			continue
		}

		// Keep non key=value lines (network block braces, etc.)
		sb.WriteString(line + "\n")
	}

	// Second pass: add new parameters only if there are any
	hasNew := false
	for key := range configMap {
		if !updatedKeys[key] {
			hasNew = true
			break
		}
	}
	if hasNew {
		sb.WriteString("\n# Additional parameters\n")
		for key, value := range configMap {
			if !updatedKeys[key] {
				sb.WriteString(fmt.Sprintf("%s=%s\n", key, value))
			}
		}
	}

	return sb.String()
}

// saveConfig saves the configuration to file
func (e *WpaEditor) saveConfig(content string) error {
	// Ensure directory exists
	dir := filepath.Dir(e.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// If mkdir fails due to read-only fs, try remounting
		exec.Command("mount", "-o", "remount,rw", "/").Run()
		if err2 := os.MkdirAll(dir, 0755); err2 != nil {
			return fmt.Errorf(i18n.T("wpa_ed.mkdir_fail"), err2)
		}
	}

	// Write file
	if err := os.WriteFile(e.configPath, []byte(content), 0600); err != nil {
		// Try remounting root as rw for embedded Linux with read-only rootfs
		exec.Command("mount", "-o", "remount,rw", "/").Run()
		if err2 := os.WriteFile(e.configPath, []byte(content), 0600); err2 != nil {
			return fmt.Errorf(i18n.T("wpa_ed.write_fail"), err2)
		}
	}

	return nil
}

// restartNetworkIfNeeded checks if wpa_supplicant is running and restarts the network
func (e *WpaEditor) restartNetworkIfNeeded() error {
	fmt.Println("\n" + i18n.T("wpa_ed.net_check"))

	// Check if wpa_supplicant is running
	checkCmd := exec.Command("pgrep", "-x", "wpa_supplicant")
	output, err := checkCmd.Output()

	if err != nil || len(output) == 0 {
		fmt.Println(i18n.T("wpa_ed.net_not_running"))
		return nil
	}

	fmt.Println(i18n.T("wpa_ed.net_running"))
	fmt.Print(i18n.T("wpa_ed.net_restart_ask"))

	response, _ := e.reader.ReadString('\n')
	response = strings.TrimSpace(response)

	if response != "y" && response != "Y" {
		fmt.Println(i18n.T("wpa_ed.net_skip"))
		return nil
	}

	// Restart network interface
	iface := e.module.Kernel.Interface
	fmt.Printf(i18n.Tf("wpa_ed.net_restarting", iface)+"\n")

	// Find necessary commands
	ipPath := sys.FindCommand("ip", "/sbin/ip")
	pkillPath := sys.FindCommand("pkill", "/usr/bin/pkill")

	// Stop wpa_supplicant
	exec.Command(pkillPath, "-9", "wpa_supplicant").Run()

	// Bring interface down and up
	exec.Command(ipPath, "link", "set", iface, "down").Run()
	exec.Command(ipPath, "link", "set", iface, "up").Run()

	fmt.Println(i18n.T("wpa_ed.net_restarted"))
	return nil
}
