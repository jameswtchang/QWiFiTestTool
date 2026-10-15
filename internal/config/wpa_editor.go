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
	ctrlInterface := ""
	if e.platformInfo != nil {
		ctrlInterface = e.platformInfo.WpaSupplicantSocket
	}
	return GenerateWpaSupplicantConfig(e.module, ctrlInterface, "", "")
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
	config := parseConfig(content)
	reader := e.reader

	for {
		// Display current config state
		fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("wpa_ed.params_title") + " ==========" + ui.Reset)
		e.displayConfigMap(config)
		fmt.Println(ui.Blue + ui.Bold + "==================================" + ui.Reset)

		// Display parameter menu
		fmt.Println("\n" + ui.Yellow + ui.Bold + "========== " + i18n.T("wpa_ed.editable_title") + " ==========" + ui.Reset)
		for i, param := range params {
			if currentVal, exists := config.GlobalParams[param.Key]; exists {
				fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %-20s ("+i18n.T("wpa_ed.current_val")+ui.Green+"%s"+ui.Reset+")\n", i+1, param.Key, currentVal)
			} else {
				fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %-20s ("+ui.Yellow+i18n.T("wpa_ed.not_set")+ui.Reset+")\n", i+1, param.Key)
			}
		}

		idxBandLock := len(params) + 1
		idxProtoLimit := len(params) + 2
		idxBwLimit := len(params) + 3

		fmt.Println(ui.Cyan + i18n.T("wpa_ed.section_limit") + ui.Reset)

		bandLockStatus := i18n.T("wpa_ed.status_auto")
		protoLimitStatus := i18n.T("wpa_ed.status_auto")
		bwLimitStatus := i18n.T("wpa_ed.status_auto")

		if len(config.Networks) > 0 {
			net := config.Networks[0]
			if _, ok := net.Params["freq_list"]; ok {
				bandLockStatus = i18n.T("wpa_ed.status_set")
			}
			if net.Params["disable_eht"] != "" || net.Params["disable_he"] != "" || net.Params["disable_vht"] != "" {
				protoLimitStatus = i18n.T("wpa_ed.status_limited")
			}
			if net.Params["disable_ht40"] != "" {
				bwLimitStatus = i18n.T("wpa_ed.status_bw_20m")
			}
		}

		fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %-20s ("+ui.Yellow+"%s"+ui.Reset+")\n", idxBandLock, i18n.T("wpa_ed.band_lock"), bandLockStatus)
		fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %-20s ("+ui.Yellow+"%s"+ui.Reset+")\n", idxProtoLimit, i18n.T("wpa_ed.proto_limit"), protoLimitStatus)
		fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %-20s ("+ui.Yellow+"%s"+ui.Reset+")\n", idxBwLimit, i18n.T("wpa_ed.bw_limit"), bwLimitStatus)

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

		if idx < 1 || idx > idxBwLimit {
			fmt.Println(i18n.T("wpa_ed.invalid_sel"))
			continue
		}

		if idx == idxBandLock {
			e.handleBandLock(config)
			continue
		} else if idx == idxProtoLimit {
			e.handleProtoLimit(config)
			continue
		} else if idx == idxBwLimit {
			e.handleBwLimit(config)
			continue
		}

		param := params[idx-1]

		// Show parameter details
		fmt.Printf("\n=== %s ===\n", param.Key)
		fmt.Printf(i18n.T("wpa.param.desc_label")+"%s\n", param.Description())
		fmt.Printf(i18n.T("wpa.param.suggest")+"%v\n", param.SuggestedValues())
		fmt.Printf(i18n.T("wpa.param.default")+"%s\n", param.DefaultValue)
		fmt.Printf(i18n.T("wpa.param.example")+"%s\n", param.Example)

		if current, exists := config.GlobalParams[param.Key]; exists {
			fmt.Printf(i18n.Tf("wpa_ed.current_val2", current)+"\n")
		}

		fmt.Printf("\n"+i18n.Tf("wpa_ed.input_val", param.DefaultValue))
		value, _ := reader.ReadString('\n')
		value = strings.TrimSpace(value)

		if value == "d" || value == "D" {
			delete(config.GlobalParams, param.Key)
			fmt.Printf(i18n.Tf("wpa_ed.removed", param.Key)+"\n")
		} else {
			if value == "" {
				value = param.DefaultValue
			}
			config.GlobalParams[param.Key] = value
			fmt.Printf(i18n.Tf("wpa_ed.set", param.Key, value)+"\n")
		}
	}

	newContent := rebuildConfig(content, config)
	return newContent, nil
}

func (e *WpaEditor) getFirstNetwork(config *WpaConfig) *WpaNetwork {
	if len(config.Networks) == 0 {
		net := &WpaNetwork{
			Params: map[string]string{
				"ssid":     "\"DummyAP_For_Test\"",
				"key_mgmt": "NONE",
			},
		}
		config.Networks = append(config.Networks, net)
	}
	return config.Networks[0]
}

func (e *WpaEditor) handleBandLock(config *WpaConfig) {
	fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.T("wpa_ed.band_lock_title") + ui.Reset)
	fmt.Println(ui.Cyan + "[1]" + ui.Reset + " " + i18n.T("wpa_ed.band_lock.1"))
	fmt.Println(ui.Cyan + "[2]" + ui.Reset + " " + i18n.T("wpa_ed.band_lock.2"))
	fmt.Println(ui.Cyan + "[3]" + ui.Reset + " " + i18n.T("wpa_ed.band_lock.3"))
	fmt.Println(ui.Cyan + "[4]" + ui.Reset + " " + i18n.T("wpa_ed.band_lock.4"))
	fmt.Println(ui.Cyan + "[5]" + ui.Reset + " " + i18n.T("wpa_ed.band_lock.5"))
	fmt.Println(ui.Cyan + "[0]" + ui.Reset + " " + i18n.T("wpa_ed.band_lock.0"))
	fmt.Print(ui.Yellow + i18n.T("wpa_ed.band_lock_select") + ui.Reset)

	input, _ := e.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "0" || input == "" {
		return
	}

	net := e.getFirstNetwork(config)

	if input == "1" {
		delete(net.Params, "freq_list")
		fmt.Println(i18n.T("wpa_ed.band_lock_cleared"))
		return
	}

	// Prepare parameters for channel lookup
	country := strings.ToUpper(strings.Trim(strings.TrimSpace(config.GlobalParams["country"]), "\""))
	if country == "" {
		country = "US"
	}

	db, err := models.LoadChannelLookup()
	if err != nil {
		fmt.Printf(i18n.Tf("wpa_ed.db_load_fail", err) + "\n")
		return
	}

	var freqs []string

	if input == "2" {
		// 6GHz
		channels := db.LookupChannels(country, "所有类型AP", "6G", 20)
		for _, ch := range channels {
			freqs = append(freqs, fmt.Sprintf("%d", models.ChannelToFreq("6G", ch)))
		}
	} else if input == "3" || input == "4" {
		// 5GHz
		channels := db.LookupChannels(country, "所有类型AP", "5G", 20)
		for _, ch := range channels {
			if input == "3" && models.IsDFSChannel(ch) {
				continue // 排除 DFS
			}
			freqs = append(freqs, fmt.Sprintf("%d", models.ChannelToFreq("5G", ch)))
		}
	} else if input == "5" {
		// 2.4GHz
		channels := db.LookupChannels(country, "所有类型AP", "2.4G", 20)
		for _, ch := range channels {
			freqs = append(freqs, fmt.Sprintf("%d", models.ChannelToFreq("2.4G", ch)))
		}
	} else {
		fmt.Println(i18n.T("wpa_ed.invalid_sel"))
		return
	}

	if len(freqs) == 0 {
		fmt.Println(i18n.T("wpa_ed.band_lock_no_ch"))
		return
	}

	net.Params["freq_list"] = strings.Join(freqs, " ")
	fmt.Println(i18n.Tf("wpa_ed.band_lock_applied", len(freqs)))
}

func (e *WpaEditor) handleProtoLimit(config *WpaConfig) {
	fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.T("wpa_ed.proto_limit_title") + ui.Reset)
	fmt.Println(ui.Cyan + "[1]" + ui.Reset + " " + i18n.T("wpa_ed.proto_limit.1"))
	fmt.Println(ui.Cyan + "[2]" + ui.Reset + " " + i18n.T("wpa_ed.proto_limit.2"))
	fmt.Println(ui.Cyan + "[3]" + ui.Reset + " " + i18n.T("wpa_ed.proto_limit.3"))
	fmt.Println(ui.Cyan + "[4]" + ui.Reset + " " + i18n.T("wpa_ed.proto_limit.4"))
	fmt.Println(ui.Cyan + "[0]" + ui.Reset + " " + i18n.T("wpa_ed.proto_limit.0"))
	fmt.Print(ui.Yellow + i18n.T("wpa_ed.proto_limit_select") + ui.Reset)

	input, _ := e.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "0" || input == "" {
		return
	}

	net := e.getFirstNetwork(config)

	// 先清除舊有設定
	delete(net.Params, "disable_eht")
	delete(net.Params, "disable_he")
	delete(net.Params, "disable_vht")

	if input == "1" {
		fmt.Println(i18n.T("wpa_ed.proto_limit_cleared"))
	} else if input == "2" {
		net.Params["disable_eht"] = "1"
		fmt.Println(i18n.T("wpa_ed.proto_limit_wifi6"))
	} else if input == "3" {
		net.Params["disable_eht"] = "1"
		net.Params["disable_he"] = "1"
		fmt.Println(i18n.T("wpa_ed.proto_limit_wifi5"))
	} else if input == "4" {
		net.Params["disable_eht"] = "1"
		net.Params["disable_he"] = "1"
		net.Params["disable_vht"] = "1"
		fmt.Println(i18n.T("wpa_ed.proto_limit_wifi4"))
	} else {
		fmt.Println(i18n.T("wpa_ed.invalid_sel"))
	}
}

func (e *WpaEditor) getWpaSupplicantPath() string {
	if e.module != nil && e.module.ID == "quectel_fgh200m" {
		return sys.FindCommand("wpa_supplicant_s1g", "/usr/sbin/wpa_supplicant_s1g")
	}
	if e.platformInfo != nil && e.platformInfo.WpaSupplicantPath != "" {
		return e.platformInfo.WpaSupplicantPath
	}
	return sys.FindCommand("wpa_supplicant", "/usr/sbin/wpa_supplicant")
}

func (e *WpaEditor) handleBwLimit(config *WpaConfig) {
	binPath := e.getWpaSupplicantPath()
	ht40Supported := sys.CheckWpaBinarySupport(binPath, "disable_ht40")

	fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.T("wpa_ed.bw_limit_title") + ui.Reset)
	fmt.Println(ui.Cyan + "[1]" + ui.Reset + " " + i18n.T("wpa_ed.bw_limit.1"))
	opt2Label := i18n.T("wpa_ed.bw_limit.2")
	if !ht40Supported {
		opt2Label += " " + ui.Red + i18n.T("wpa_ed.bw_limit_unsupported_tag") + ui.Reset
	}
	fmt.Println(ui.Cyan + "[2]" + ui.Reset + " " + opt2Label)
	fmt.Println(ui.Cyan + "[0]" + ui.Reset + " " + i18n.T("wpa_ed.bw_limit.0"))
	fmt.Print(ui.Yellow + i18n.T("wpa_ed.bw_limit_select") + ui.Reset)

	input, _ := e.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "0" || input == "" {
		return
	}

	net := e.getFirstNetwork(config)

	// 先清除舊有設定
	delete(net.Params, "disable_ht40")

	if input == "1" {
		fmt.Println(i18n.T("wpa_ed.bw_limit_cleared"))
	} else if input == "2" {
		if !ht40Supported {
			fmt.Println(ui.Yellow + "[WARN] " + i18n.Tf("wpa_ed.bw_limit_unsupported_warn", binPath) + ui.Reset)
			fmt.Println(ui.Yellow + "[WARN] " + i18n.T("wpa_ed.bw_limit_unsupported_hint") + ui.Reset)
			fmt.Print(ui.Red + i18n.T("wpa_ed.bw_limit_force_ask") + " [y/N]: " + ui.Reset)
			confirm, _ := e.reader.ReadString('\n')
			confirm = strings.TrimSpace(confirm)
			if strings.ToLower(confirm) != "y" {
				fmt.Println(i18n.T("wpa_ed.bw_limit_cancelled"))
				return
			}
		}
		net.Params["disable_ht40"] = "1"
		fmt.Println(i18n.T("wpa_ed.bw_limit_20m"))
	} else {
		fmt.Println(i18n.T("wpa_ed.invalid_sel"))
	}
}

// displayConfigMap shows current config key-value pairs in order
func (e *WpaEditor) displayConfigMap(config *WpaConfig) {
	if len(config.GlobalParams) == 0 && len(config.Networks) == 0 {
		fmt.Println("  " + i18n.T("wpa_ed.empty"))
		return
	}

	// Show known params first in order
	params := GetWpaParams()
	shown := make(map[string]bool)

	for _, param := range params {
		if val, exists := config.GlobalParams[param.Key]; exists {
			fmt.Printf("  %s=%s\n", param.Key, val)
			shown[param.Key] = true
		}
	}

	// Show remaining global params (ctrl_interface, etc.)
	for key, val := range config.GlobalParams {
		if !shown[key] {
			fmt.Printf("  %s=%s\n", key, val)
		}
	}

	// Show network params
	binPath := e.getWpaSupplicantPath()
	var unsupportedFound []string
	for i, net := range config.Networks {
		ssid := net.Params["ssid"]
		if ssid == "" {
			ssid = "<unknown>"
		}
		fmt.Printf("\n  [Network %d: %s]\n", i+1, ssid)
		for key, val := range net.Params {
			if key != "ssid" {
				tag := ""
				if strings.HasPrefix(key, "disable_") {
					if !sys.CheckWpaBinarySupport(binPath, key) {
						tag = " " + ui.Red + i18n.T("wpa_ed.unsupported_tag") + ui.Reset
						unsupportedFound = append(unsupportedFound, fmt.Sprintf("%s=%s", key, val))
					}
				}
				fmt.Printf("    %s=%s%s\n", key, val, tag)
			}
		}
	}
	if len(unsupportedFound) > 0 {
		fmt.Println("\n" + ui.Yellow + i18n.Tf("wpa_ed.unsupported_param_warn", strings.Join(unsupportedFound, ", ")) + ui.Reset)
	}
}

// WpaNetwork represents a parsed network block
type WpaNetwork struct {
	Params map[string]string
}

// WpaConfig represents the parsed wpa_supplicant.conf
type WpaConfig struct {
	GlobalParams map[string]string
	Networks     []*WpaNetwork
}

// parseConfig parses config content into a structured WpaConfig
func parseConfig(content string) *WpaConfig {
	config := &WpaConfig{
		GlobalParams: make(map[string]string),
		Networks:     make([]*WpaNetwork, 0),
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentNetwork *WpaNetwork

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		noSpace := strings.ReplaceAll(line, " ", "")
		if strings.HasPrefix(noSpace, "network={") {
			currentNetwork = &WpaNetwork{
				Params: make(map[string]string),
			}
			config.Networks = append(config.Networks, currentNetwork)
			continue
		}

		if (line == "}" || strings.HasPrefix(line, "}")) && currentNetwork != nil {
			currentNetwork = nil
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if currentNetwork != nil {
				currentNetwork.Params[k] = v
			} else {
				config.GlobalParams[k] = v
			}
		}
	}

	return config
}

// rebuildConfig rebuilds the config file with updated parameters
func rebuildConfig(originalContent string, config *WpaConfig) string {
	var sb strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(originalContent))
	
	updatedGlobalKeys := make(map[string]bool)
	
	inNetwork := false
	networkIdx := -1
	var currentNetworkKeys map[string]bool

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "# Additional parameters" {
			continue
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			sb.WriteString(line + "\n")
			continue
		}

		noSpace := strings.ReplaceAll(trimmed, " ", "")
		if strings.HasPrefix(noSpace, "network={") {
			inNetwork = true
			networkIdx++
			currentNetworkKeys = make(map[string]bool)
			sb.WriteString(line + "\n")
			continue
		}

		if (trimmed == "}" || strings.HasPrefix(trimmed, "}")) && inNetwork {
			if networkIdx >= 0 && networkIdx < len(config.Networks) {
				net := config.Networks[networkIdx]
				for k, v := range net.Params {
					if !currentNetworkKeys[k] {
						sb.WriteString(fmt.Sprintf("\t%s=%s\n", k, v))
					}
				}
			}
			sb.WriteString(line + "\n")
			inNetwork = false
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			
			if inNetwork {
				if currentNetworkKeys[key] {
					continue
				}
				if networkIdx >= 0 && networkIdx < len(config.Networks) {
					net := config.Networks[networkIdx]
					if newValue, exists := net.Params[key]; exists {
						sb.WriteString(fmt.Sprintf("\t%s=%s\n", key, newValue))
						currentNetworkKeys[key] = true
					}
				} else {
					sb.WriteString(line + "\n")
				}
			} else {
				if updatedGlobalKeys[key] {
					continue
				}
				if newValue, exists := config.GlobalParams[key]; exists {
					sb.WriteString(fmt.Sprintf("%s=%s\n", key, newValue))
					updatedGlobalKeys[key] = true
				}
			}
			continue
		}

		sb.WriteString(line + "\n")
	}

	// Add new networks
	for i := networkIdx + 1; i < len(config.Networks); i++ {
		sb.WriteString("\nnetwork={\n")
		net := config.Networks[i]
		for k, v := range net.Params {
			sb.WriteString(fmt.Sprintf("\t%s=%s\n", k, v))
		}
		sb.WriteString("}\n")
	}

	// Add new global params
	hasNew := false
	for key := range config.GlobalParams {
		if !updatedGlobalKeys[key] {
			hasNew = true
			break
		}
	}
	if hasNew {
		sb.WriteString("\n# Additional parameters\n")
		for key, value := range config.GlobalParams {
			if !updatedGlobalKeys[key] {
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

	// Check if a wpa_supplicant that may control this interface is running
	// (bound to it, or global/D-Bus mode); instances on other interfaces are ignored
	iface := e.module.Kernel.GetInterface()
	supplicants := []string{"wpa_supplicant", "wpa_supplicant_s1g"}
	if len(sys.FindDaemonsForIface(supplicants, iface)) == 0 {
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
	fmt.Printf(i18n.Tf("wpa_ed.net_restarting", iface)+"\n")

	ipPath := sys.FindCommand("ip", "/sbin/ip")

	// Stop wpa_supplicant on this interface only
	sys.KillDaemonsForIface(supplicants, iface)

	// Bring interface down and up
	exec.Command(ipPath, "link", "set", iface, "down").Run()
	exec.Command(ipPath, "link", "set", iface, "up").Run()

	fmt.Println(i18n.T("wpa_ed.net_restarted"))
	return nil
}
