package config

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/ui"
)

// APEditorParams holds the current AP configuration state
type APEditorParams struct {
	CountryCode string
	APType      string // Indoor AP, Outdoor AP
	Band        string // 2.4G, 5G
	Mode        string // n, ac, ax
	Bandwidth   int    // 20, 40, 80, 160
	Channel     int
	SSID        string
	Security    string // Open, WPA2, WPA3
	Password    string
}

// APConfigEditor handles the interactive AP config generation TUI
type APConfigEditor struct {
	module    *models.WiFiModule
	channelDB *models.ChannelLookupDB
	params    APEditorParams
	reader    *bufio.Reader
}

// NewAPConfigEditor creates a new AP config editor
func NewAPConfigEditor(module *models.WiFiModule) (*APConfigEditor, error) {
	db, err := models.LoadChannelLookup()
	if err != nil {
		return nil, fmt.Errorf(i18n.T("ap_ed.db_load_fail"), err)
	}

	editor := &APConfigEditor{
		module:    module,
		channelDB: db,
		reader:    bufio.NewReader(os.Stdin),
	}

	// Initialize with defaults from XML APProfile
	editor.loadDefaults()

	return editor, nil
}

// loadDefaults initializes params from XML APProfile
func (e *APConfigEditor) loadDefaults() {
	e.params = APEditorParams{
		CountryCode: "TW",
		APType:      i18n.T("ap_ed.indoor_ap"),
		Band:        "2.4G",
		Mode:        "n",
		Bandwidth:   20,
		Channel:     6,
		SSID:        e.module.DisplayName + "_AP",
		Security:    "WPA2",
		Password:    "12345678",
	}

	if p := e.module.APConfig.APProfile; p != nil {
		if p.SSID != "" {
			e.params.SSID = p.SSID
		}
		if p.Security != "" {
			e.params.Security = p.Security
		}
		if p.Password != "" {
			e.params.Password = p.Password
		}
		if p.Band != "" {
			e.params.Band = p.Band
		}
		if p.Channel > 0 {
			e.params.Channel = p.Channel
		}
	}

	// Set mode based on capabilities
	caps := e.module.APConfig.Capabilities
	if caps != nil {
		if caps.HasMode("ah") {
			e.params.Mode = "ah"
		} else if caps.HasMode("be") {
			e.params.Mode = "be"
		} else if caps.HasMode("ax") {
			e.params.Mode = "ax"
		} else if caps.HasMode("ac") {
			e.params.Mode = "ac"
		} else {
			e.params.Mode = "n"
		}
		if caps.MaxBandwidth > 0 {
			e.params.Bandwidth = caps.MaxBandwidth
		}
	}

	// Try to get default CountryCode from FixedParams or Params
	var allParams []models.Param
	allParams = append(allParams, e.module.APConfig.FixedParams...)
	allParams = append(allParams, e.module.APConfig.Params...)
	for _, p := range allParams {
		if p.Key == "country_code" {
			e.params.CountryCode = p.Value
			break
		}
	}

	// Fallback: parse country code from InsmodArgs (e.g. rtw_country_code=CN)
	if e.params.CountryCode == "TW" && e.module.Kernel.InsmodArgs != "" {
		for _, arg := range strings.Split(e.module.Kernel.InsmodArgs, " ") {
			if strings.HasPrefix(arg, "rtw_country_code=") {
				e.params.CountryCode = strings.TrimPrefix(arg, "rtw_country_code=")
				break
			}
		}
	}
}

// Start begins the interactive editing session
func (e *APConfigEditor) Start() error {
	fmt.Println("\n" + ui.Cyan + ui.Bold + "╔═══════════════════════════════════════════════════╗" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "║     " + i18n.T("ap_ed.title") + "              ║" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "╚═══════════════════════════════════════════════════╝" + ui.Reset)
	fmt.Println(i18n.Tf("ap_ed.module_label", e.module.DisplayName))

	// Check if system config exists
	hostapdPath := "/etc/hostapd.conf"
	if fileExists(hostapdPath) {
		fmt.Printf("\n"+ui.Cyan+"[INFO] "+ui.Reset+i18n.Tf("ap_ed.sys_config_found", hostapdPath)+"\n")
		e.parseExistingHostapd(hostapdPath)
	} else {
		fmt.Println("\n" + ui.Cyan + "[INFO] " + ui.Reset + i18n.T("ap_ed.sys_config_none"))
	}

	for {
		e.showCurrentConfig()

		fmt.Println(ui.Cyan + ui.Bold + "[1]" + ui.Reset + " " + i18n.T("ap_ed.action.1"))
		fmt.Println(ui.Cyan + ui.Bold + "[2]" + ui.Reset + " " + i18n.T("ap_ed.action.2"))
		fmt.Println(ui.Cyan + ui.Bold + "[3]" + ui.Reset + " " + i18n.T("ap_ed.action.3"))
		fmt.Println(ui.Cyan + ui.Bold + "[4]" + ui.Reset + " " + i18n.T("ap_ed.action.4"))
		fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " " + i18n.T("ap_ed.action.0"))
		fmt.Print(ui.Yellow + ui.Bold + i18n.T("ap_ed.action.select") + ui.Reset)

		input := e.readLine()

		switch input {
		case "1":
			e.editParams()
			saved, err := e.generateAndPreview()
			if err != nil {
				fmt.Printf("\n"+i18n.Tf("err.prefix", err)+"\n")
			} else if saved {
				return nil
			}
		case "2":
			e.loadDefaults()
			fmt.Println(i18n.T("ap_ed.defaults_restored"))
			saved, err := e.generateAndPreview()
			if err != nil {
				fmt.Printf("\n"+i18n.Tf("err.prefix", err)+"\n")
			} else if saved {
				return nil
			}
		case "3":
			loaded, err := e.loadProfileTemplate()
			if err != nil {
				fmt.Printf("\n"+i18n.Tf("ap_ed.tpl_load_fail", err)+"\n")
			} else if loaded {
				saved, err := e.generateAndPreview()
				if err != nil {
					fmt.Printf("\n"+i18n.Tf("err.prefix", err)+"\n")
				} else if saved {
					return nil
				}
			}
		case "4":
			if e.runQuickPresetWizard() {
				saved, err := e.generateAndPreview()
				if err != nil {
					fmt.Printf("\n"+i18n.Tf("err.prefix", err)+"\n")
				} else if saved {
					return nil
				}
			}
		case "0":
			return nil
		default:
			fmt.Println(i18n.T("common.invalid"))
		}
	}
}

// parseExistingHostapd reads an existing hostapd.conf and populates editor params
func (e *APConfigEditor) parseExistingHostapd(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	configMap := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			configMap[key] = value
		}
	}

	if v, ok := configMap["ssid"]; ok {
		e.params.SSID = v
	}
	if v, ok := configMap["country_code"]; ok {
		e.params.CountryCode = v
	}
	if v, ok := configMap["channel"]; ok {
		if ch, err := strconv.Atoi(v); err == nil {
			e.params.Channel = ch
		}
	}

	// hw_mode → Band
	if v, ok := configMap["hw_mode"]; ok {
		if configMap["op_class"] == "131" || configMap["op_class"] == "132" || configMap["op_class"] == "133" || configMap["op_class"] == "134" || configMap["op_class"] == "135" || configMap["op_class"] == "136" || configMap["op_class"] == "137" || (v == "a" && configMap["channel"] == "37") {
			e.params.Band = "6G"
		} else if v == "a" {
			e.params.Band = "5G"
		} else {
			e.params.Band = "2.4G"
		}
	}

	// ieee80211 flags → Mode
	if configMap["ieee80211ah"] == "1" {
		e.params.Mode = "ah"
		e.params.Band = "sub-GHz"
	} else if configMap["ieee80211be"] == "1" {
		e.params.Mode = "be"
	} else if configMap["ieee80211ax"] == "1" {
		e.params.Mode = "ax"
	} else if configMap["ieee80211ac"] == "1" {
		e.params.Mode = "ac"
	} else if configMap["ieee80211n"] == "1" {
		e.params.Mode = "n"
	}

	// ht_capab / op_class → Bandwidth
	if ht, ok := configMap["ht_capab"]; ok {
		if strings.Contains(ht, "HT40") {
			e.params.Bandwidth = 40
		} else {
			e.params.Bandwidth = 20
		}
	}
	if configMap["vht_oper_chwidth"] == "1" || configMap["he_oper_chwidth"] == "1" || configMap["eht_oper_chwidth"] == "1" {
		e.params.Bandwidth = 80
	}
	if configMap["vht_oper_chwidth"] == "2" || configMap["he_oper_chwidth"] == "2" || configMap["eht_oper_chwidth"] == "2" {
		e.params.Bandwidth = 160
	}
	if configMap["eht_oper_chwidth"] == "3" {
		e.params.Bandwidth = 320
	}
	// In 6GHz, bandwidth is directly indicated by op_class
	if op, ok := configMap["op_class"]; ok {
		switch op {
		case "131":
			e.params.Bandwidth = 20
		case "132":
			e.params.Bandwidth = 40
		case "133":
			e.params.Bandwidth = 80
		case "134":
			e.params.Bandwidth = 160
		case "135": // 80+80 MHz (deprecated in WiFi 7, treat as 160)
			e.params.Bandwidth = 160
		case "136": // 20 MHz (6GHz discovery channel)
			e.params.Bandwidth = 20
		case "137":
			e.params.Bandwidth = 320
		}
	}

	// Security
	if configMap["wpa"] == "2" {
		if configMap["wpa_key_mgmt"] == "SAE" {
			e.params.Security = "WPA3"
		} else {
			e.params.Security = "WPA2"
		}
		if v, ok := configMap["sae_password"]; ok {
			e.params.Password = v
		} else if v, ok := configMap["wpa_passphrase"]; ok {
			e.params.Password = v
		}
	} else {
		e.params.Security = "Open"
		e.params.Password = ""
	}

	// 6GHz: restore center channel instead of primary channel
	// hostapd stores channel= as the 20MHz primary, but our TUI uses center channel IDs
	if e.params.Band == "6G" && e.params.Bandwidth > 20 {
		if v, ok := configMap["eht_oper_centr_freq_seg0_idx"]; ok {
			if ch, err := strconv.Atoi(v); err == nil {
				e.params.Channel = ch
			}
		} else if v, ok := configMap["he_oper_centr_freq_seg0_idx"]; ok {
			if ch, err := strconv.Atoi(v); err == nil {
				e.params.Channel = ch
			}
		}
	}
}

// showCurrentConfig displays the current parameter state
func (e *APConfigEditor) showCurrentConfig() {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("ap_ed.current_hostapd") + " ==========" + ui.Reset)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.country"), 14), e.params.CountryCode)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.ap_type"), 14), e.params.APType)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.band"), 14), e.params.Band)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.mode"), 14), e.modeDisplayName(e.params.Mode))
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%dMHz"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.bandwidth"), 14), e.params.Bandwidth)
	if (e.params.Band == "6G" || e.params.Band == "6g") && e.params.Bandwidth > 20 {
		centerCh := e.calcHE6GCenterChannel()
		primaryCh := CalcPrimaryChannel6G(centerCh, e.params.Bandwidth)
		if centerCh > 0 && isValidPSCForBlock(e.params.Channel, centerCh, e.params.Bandwidth) {
			primaryCh = e.params.Channel
		}
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"Center: %d, Primary: %d (PSC)"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.channel"), 14), centerCh, primaryCh)
	} else {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%d"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.channel"), 14), e.params.Channel)
	}
	fmt.Printf(ui.Bold+"  %s "+ui.Cyan+"%s"+ui.Reset+"\n", ui.AlignLabel("SSID:", 14), e.params.SSID)
	fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.security"), 14), e.params.Security)
	if e.params.Security != "Open" {
		fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("ap_ed.label.password"), 14), e.params.Password)
	}

	gateway := "192.168.50.1"
	dhcpStart := "192.168.50.100"
	dhcpEnd := "192.168.50.200"

	if net := e.module.APConfig.Network; net != nil {
		if net.Gateway != "" {
			gateway = net.Gateway
		}
		if net.DHCPStart != "" {
			dhcpStart = net.DHCPStart
		}
		if net.DHCPEnd != "" {
			dhcpEnd = net.DHCPEnd
		}
	}

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("ap_ed.current_dnsmasq") + " ==========" + ui.Reset)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("Gateway:", 14), gateway)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s - %s"+ui.Reset+"\n", ui.AlignLabel("DHCP Range:", 14), dhcpStart, dhcpEnd)
	fmt.Println(ui.Blue + ui.Bold + "========================================" + ui.Reset)
}

func (e *APConfigEditor) modeDisplayName(mode string) string {
	switch mode {
	case "ah":
		return "WiFi HaLow (IEEE 802.11ah)"
	case "n":
		return "WiFi 4 (n)"
	case "ac":
		return "WiFi 5 (ac)"
	case "ax":
		return "WiFi 6 (ax)"
	default:
		return mode
	}
}

// editParams provides a submenu to edit individual parameters
func (e *APConfigEditor) editParams() {
	for {
		fmt.Println("\n" + ui.Yellow + ui.Bold + "---------- " + i18n.T("ap_ed.edit_title") + " ----------" + ui.Reset)
		fmt.Println(ui.Cyan + ui.Bold + "[1]" + ui.Reset + " " + i18n.T("ap_ed.edit.1"))
		fmt.Println(ui.Cyan + ui.Bold + "[2]" + ui.Reset + " " + i18n.T("ap_ed.edit.2"))
		fmt.Println(ui.Cyan + ui.Bold + "[3]" + ui.Reset + " " + i18n.T("ap_ed.edit.3"))
		fmt.Println(ui.Cyan + ui.Bold + "[4]" + ui.Reset + " " + i18n.T("ap_ed.edit.4"))
		fmt.Println(ui.Cyan + ui.Bold + "[5]" + ui.Reset + " " + i18n.T("ap_ed.edit.5"))
		fmt.Println(ui.Cyan + ui.Bold + "[6]" + ui.Reset + " " + i18n.T("ap_ed.edit.6"))
		fmt.Println(ui.Cyan + ui.Bold + "[7]" + ui.Reset + " " + i18n.T("ap_ed.edit.7"))
		fmt.Println(ui.Cyan + ui.Bold + "[8]" + ui.Reset + " " + i18n.T("ap_ed.edit.8"))
		fmt.Println(ui.Cyan + ui.Bold + "[9]" + ui.Reset + " " + i18n.T("ap_ed.edit.9"))
		fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " " + i18n.T("ap_ed.edit.0"))
		fmt.Print(ui.Yellow + ui.Bold + i18n.T("ap_ed.edit.select") + ui.Reset)

		input := e.readLine()

		switch input {
		case "1":
			e.editCountryCode()
		case "2":
			e.editAPType()
		case "3":
			e.editBand()
		case "4":
			e.editMode()
		case "5":
			e.editBandwidth()
		case "6":
			e.editChannel()
		case "7":
			e.editSSID()
		case "8":
			e.editSecurity()
		case "9":
			e.editPassword()
		case "0":
			return
		default:
			fmt.Println(i18n.T("common.invalid"))
		}
	}
}

// --- Parameter editors ---

func (e *APConfigEditor) editCountryCode() {
	codes := e.channelDB.ListCountryCodes()
	fmt.Printf("\n"+i18n.Tf("ap_ed.cc.available", len(codes))+"\n")

	// Show in columns
	cols := 10
	for i, code := range codes {
		fmt.Printf("%-5s", code)
		if (i+1)%cols == 0 {
			fmt.Println()
		}
	}
	fmt.Println()

	fmt.Printf(i18n.Tf("ap_ed.cc.input", e.params.CountryCode))
	input := strings.ToUpper(e.readLine())
	if input == "" {
		return
	}

	if e.channelDB.FindCountry(input) != nil {
		e.params.CountryCode = input
		e.refreshChannels()
		fmt.Println(i18n.Tf("ap_ed.cc.set", input))
	} else {
		fmt.Println(i18n.T("ap_ed.cc.invalid"))
	}
}

func (e *APConfigEditor) editAPType() {
	country := e.channelDB.FindCountry(e.params.CountryCode)
	if country == nil {
		fmt.Println(i18n.T("ap_ed.cc.required"))
		return
	}

	types := country.GetAPTypes()
	fmt.Println("\n" + i18n.T("ap_ed.apt.available"))
	for i, t := range types {
		marker := "  "
		if t == e.params.APType {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %s\n", marker, i+1, t)
	}

	fmt.Print(i18n.T("common.select_num"))
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(types) {
		fmt.Println(i18n.T("common.invalid_sel"))
		return
	}

	e.params.APType = types[idx-1]
	e.refreshChannels()
	fmt.Println(i18n.Tf("ap_ed.apt.set", e.params.APType))
}

func (e *APConfigEditor) editBand() {
	caps := e.module.APConfig.Capabilities
	bands := []string{"2.4G"}
	if caps != nil {
		bands = caps.GetBands()
	}

	fmt.Println("\n" + i18n.T("ap_ed.band.available"))
	for i, b := range bands {
		marker := "  "
		if strings.TrimSpace(b) == e.params.Band {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %s\n", marker, i+1, strings.TrimSpace(b))
	}

	if len(bands) == 1 {
		fmt.Println(i18n.T("ap_ed.band.single"))
		e.params.Band = strings.TrimSpace(bands[0])
		return
	}

	fmt.Print(i18n.T("common.select_num"))
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(bands) {
		fmt.Println(i18n.T("common.invalid_sel"))
		return
	}

	e.params.Band = strings.TrimSpace(bands[idx-1])
	e.refreshChannels()
	fmt.Println(i18n.Tf("ap_ed.band.set", e.params.Band))
}

func (e *APConfigEditor) editMode() {
	caps := e.module.APConfig.Capabilities
	allModes := []struct {
		id      string
		display string
	}{
		{"ah", "WiFi HaLow (IEEE 802.11ah)"},
		{"n", "WiFi 4 (n)"},
		{"ac", "WiFi 5 (ac)"},
		{"ax", "WiFi 6 (ax)"},
		{"be", "WiFi 7 (be)"},
	}

	fmt.Println("\n" + i18n.T("ap_ed.mode.available"))
	var available []struct {
		id      string
		display string
	}

	for _, m := range allModes {
		supported := caps == nil || caps.HasMode(m.id)
		if supported {
			available = append(available, m)
		}
	}

	for i, m := range available {
		marker := "  "
		if m.id == e.params.Mode {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %s\n", marker, i+1, m.display)
	}

	fmt.Print(i18n.T("common.select_num"))
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(available) {
		fmt.Println(i18n.T("common.invalid_sel"))
		return
	}

	e.params.Mode = available[idx-1].id
	fmt.Println(i18n.Tf("ap_ed.mode.set", available[idx-1].display))
}

func (e *APConfigEditor) editBandwidth() {
	caps := e.module.APConfig.Capabilities
	maxBW := 320
	if caps != nil && caps.MaxBandwidth > 0 {
		maxBW = caps.MaxBandwidth
	}

	allBW := []int{20, 40, 80, 160, 320}
	fmt.Println("\n" + i18n.T("ap_ed.bw.available"))
	var available []int

	for _, bw := range allBW {
		if bw <= maxBW {
			available = append(available, bw)
		}
	}

	for i, bw := range available {
		marker := "  "
		if bw == e.params.Bandwidth {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %dMHz\n", marker, i+1, bw)
	}

	fmt.Print(i18n.T("common.select_num"))
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(available) {
		fmt.Println(i18n.T("common.invalid_sel"))
		return
	}

	e.params.Bandwidth = available[idx-1]
	e.refreshChannels()
	fmt.Println(i18n.Tf("ap_ed.bw.set", available[idx-1]))
}

func (e *APConfigEditor) editChannel() {
	channels := e.channelDB.LookupChannels(
		e.params.CountryCode, e.params.APType, e.params.Band, e.params.Bandwidth,
	)

	if len(channels) == 0 {
		fmt.Println(i18n.T("ap_ed.ch.none"))
		return
	}

	fmt.Println("\n" + i18n.T("ap_ed.ch.available"))
	cols := 10
	for i, ch := range channels {
		if ch == e.params.Channel {
			fmt.Printf("[%d]* ", ch)
		} else {
			fmt.Printf("[%d]  ", ch)
		}
		if (i+1)%cols == 0 {
			fmt.Println()
		}
	}
	fmt.Println()

	fmt.Printf(i18n.Tf("ap_ed.ch.input", e.params.Channel))
	input := e.readLine()
	if input == "" {
		return
	}

	ch, err := strconv.Atoi(input)
	if err != nil {
		fmt.Println(i18n.T("ap_ed.ch.invalid"))
		return
	}

	// Validate
	valid := false
	for _, c := range channels {
		if c == ch {
			valid = true
			break
		}
	}
	if !valid {
		fmt.Println(i18n.T("ap_ed.ch.not_in_list"))
	}

	e.params.Channel = ch
	fmt.Println(i18n.Tf("ap_ed.ch.set", ch))
}

func (e *APConfigEditor) editSSID() {
	fmt.Printf(i18n.Tf("ap_ed.ssid.input", e.params.SSID))
	input := e.readLine()
	if input != "" {
		e.params.SSID = input
		fmt.Println(i18n.Tf("ap_ed.ssid.set", input))
	}
}

func (e *APConfigEditor) editSecurity() {
	options := []string{"Open", "WPA2"}
	caps := e.module.APConfig.Capabilities
	if caps != nil && caps.SupportWPA3 {
		options = append(options, "WPA3")
	}

	fmt.Println("\n" + i18n.T("ap_ed.sec.available"))
	for i, opt := range options {
		marker := "  "
		if opt == e.params.Security {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %s\n", marker, i+1, opt)
	}

	fmt.Print(i18n.T("common.select_num"))
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(options) {
		fmt.Println(i18n.T("common.invalid_sel"))
		return
	}

	e.params.Security = options[idx-1]
	if e.params.Security == "Open" {
		e.params.Password = ""
	}
	fmt.Println(i18n.Tf("ap_ed.sec.set", options[idx-1]))
}

func (e *APConfigEditor) editPassword() {
	if e.params.Security == "Open" {
		fmt.Println(i18n.T("ap_ed.pw.open_no_need"))
		return
	}

	for {
		fmt.Printf(i18n.Tf("ap_ed.pw.input", e.params.Password))
		input := e.readLine()

		if input == "" {
			break
		}

		if len(input) < 8 || len(input) > 63 {
			fmt.Println(i18n.T("ap_ed.pw.length_err"))
			continue
		}

		e.params.Password = input
		fmt.Println(i18n.T("ap_ed.pw.set"))
		break
	}
}

// refreshChannels updates the channel when band/country/bandwidth changes
func (e *APConfigEditor) refreshChannels() {
	channels := e.channelDB.LookupChannels(
		e.params.CountryCode, e.params.APType, e.params.Band, e.params.Bandwidth,
	)
	if len(channels) > 0 {
		// Check if current channel is still valid
		valid := false
		for _, ch := range channels {
			if ch == e.params.Channel {
				valid = true
				break
			}
		}
		if !valid {
			e.params.Channel = channels[0]
			fmt.Println(i18n.Tf("ap_ed.ch.auto_adjust", e.params.Channel))
		}
	}
}

// --- Generate & Preview ---

func (e *APConfigEditor) generateAndPreview() (bool, error) {
	hostapdContent := e.generateHostapd()
	dnsmasqContent := e.generateDnsmasq()

	// Use system default config paths (same as ConfigManager tier 2)
	hostapdPath := "/etc/hostapd.conf"
	dnsmasqPath := "/etc/dnsmasq.conf"

	// Show preview
	fmt.Println("\n" + ui.Cyan + ui.Bold + "╔═══════════════════════════════════════════════════╗" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "║              " + i18n.T("ap_ed.preview.hostapd") + "                    ║" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "╚═══════════════════════════════════════════════════╝" + ui.Reset)
	fmt.Println(ui.Green + hostapdContent + ui.Reset)

	fmt.Println(ui.Cyan + ui.Bold + "╔═══════════════════════════════════════════════════╗" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "║              " + i18n.T("ap_ed.preview.dnsmasq") + "                    ║" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "╚═══════════════════════════════════════════════════╝" + ui.Reset)
	fmt.Println(ui.Green + dnsmasqContent + ui.Reset)

	// Check if files already exist
	hostapdExists := fileExists(hostapdPath)
	dnsmasqExists := fileExists(dnsmasqPath)

	if hostapdExists || dnsmasqExists {
		fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.T("ap_ed.overwrite_warn") + ui.Reset)
		if hostapdExists {
			fmt.Printf("  - %s\n", hostapdPath)
		}
		if dnsmasqExists {
			fmt.Printf("  - %s\n", dnsmasqPath)
		}
	}

	// Prompt to save
	fmt.Printf("\n" + ui.Yellow + ui.Bold + i18n.T("ap_ed.save_confirm") + ui.Reset)
	if response := e.readLine(); response == "y" || response == "Y" {
		if err := os.WriteFile(hostapdPath, []byte(hostapdContent), 0644); err != nil {
			exec.Command("mount", "-o", "remount,rw", "/").Run()
			if err2 := os.WriteFile(hostapdPath, []byte(hostapdContent), 0644); err2 != nil {
				return false, fmt.Errorf(i18n.T("ap_ed.write_fail"), hostapdPath, err2)
			}
		}
		if err := os.WriteFile(dnsmasqPath, []byte(dnsmasqContent), 0644); err != nil {
			exec.Command("mount", "-o", "remount,rw", "/").Run()
			if err2 := os.WriteFile(dnsmasqPath, []byte(dnsmasqContent), 0644); err2 != nil {
				return false, fmt.Errorf(i18n.T("ap_ed.write_fail"), dnsmasqPath, err2)
			}
		}
		fmt.Println("\n" + ui.Green + ui.Bold + i18n.T("ap_ed.saved") + ui.Reset)
		fmt.Printf("  - %s\n", hostapdPath)
		fmt.Printf("  - %s\n", dnsmasqPath)
		fmt.Println(ui.Cyan + "[INFO] " + ui.Reset + i18n.T("ap_ed.saved_hint") + "\n")

		e.showCurrentConfig()

		return true, nil
	}

	return false, nil
}

// generateHostapd generates hostapd.conf content from current params
func (e *APConfigEditor) generateHostapd() string {
	var sb strings.Builder

	sb.WriteString("# hostapd configuration\n")
	sb.WriteString(fmt.Sprintf("# Module: %s\n", e.module.DisplayName))
	sb.WriteString("# Generated by QWiFiTestTool\n\n")

	// Interface & driver from Kernel config
	sb.WriteString(fmt.Sprintf("interface=%s\n", e.module.Kernel.GetInterface()))
	sb.WriteString(fmt.Sprintf("driver=%s\n", e.module.Kernel.GetDriverBackend()))
	ctrlInterface := "/var/run/hostapd"
	ctrlGroup := "0"
	if e.module != nil {
		for _, p := range e.module.APConfig.FixedParams {
			if p.Key == "ctrl_interface" && p.Value != "" {
				ctrlInterface = p.Value
			}
			if p.Key == "ctrl_interface_group" && p.Value != "" {
				ctrlGroup = p.Value
			}
		}
	}
	sb.WriteString(fmt.Sprintf("ctrl_interface=%s\n", ctrlInterface))
	sb.WriteString(fmt.Sprintf("ctrl_interface_group=%s\n", ctrlGroup))

	// SSID
	sb.WriteString(fmt.Sprintf("ssid=%s\n", e.params.SSID))

	// Country — skip if driver handles regulatory via rtw_country_code
	countryWritten := false
	if !strings.Contains(e.module.Kernel.InsmodArgs, "rtw_country_code") {
		sb.WriteString(fmt.Sprintf("country_code=%s\n", e.params.CountryCode))
		countryWritten = true
		if e.params.Band != "sub-GHz" {
			sb.WriteString("ieee80211d=1\n")
		}
	}

	// Band → hw_mode
	if e.params.Band == "5G" || e.params.Band == "6G" || e.params.Band == "sub-GHz" {
		sb.WriteString("hw_mode=a\n")
	} else {
		sb.WriteString("hw_mode=g\n")
	}

	// Channel
	primaryChannel := e.params.Channel
	var centerCh int
	if (e.params.Band == "6G" || e.params.Band == "6g") && e.params.Bandwidth > 20 {
		centerCh = e.calcHE6GCenterChannel()
		if centerCh > 0 {
			if isValidPSCForBlock(e.params.Channel, centerCh, e.params.Bandwidth) {
				primaryChannel = e.params.Channel
			} else {
				primaryChannel = CalcPrimaryChannel6G(centerCh, e.params.Bandwidth)
			}
		}
	} else if (e.params.Band == "5G" || e.params.Band == "5g") && e.params.Bandwidth >= 80 {
		centerCh = e.calcVHTCenterChannel()
	}
	sb.WriteString(fmt.Sprintf("channel=%d\n", primaryChannel))

	// DFS check for 5GHz: hostapd_config_check() hard-requires
	// ieee80211h=1 -> ieee80211d=1 -> country_code, otherwise hostapd refuses to start.
	// Realtek (rtw_country_code) skips country_code above, so it must be written here;
	// e.params.CountryCode is already synced from rtw_country_code, so no conflict with the driver.
	if isDFSChannel(e.params.Band, e.params.Channel, centerCh, e.params.Bandwidth) {
		if !countryWritten {
			sb.WriteString(fmt.Sprintf("country_code=%s\n", e.params.CountryCode))
			sb.WriteString("ieee80211d=1\n")
			countryWritten = true
		}
		sb.WriteString("ieee80211h=1\n")
	}

	// Mode → ieee80211 flags
	switch e.params.Mode {
	case "ah":
		sb.WriteString("ieee80211ah=1\n")
	case "be":
		sb.WriteString("ieee80211be=1\n")
		sb.WriteString("ieee80211ax=1\n")
		if e.params.Band == "5G" {
			sb.WriteString("ieee80211ac=1\n")
			sb.WriteString("ieee80211n=1\n")
		} else if e.params.Band == "2.4G" {
			sb.WriteString("ieee80211n=1\n")
		}
	case "ax":
		sb.WriteString("ieee80211ax=1\n")
		if e.params.Band == "5G" {
			sb.WriteString("ieee80211ac=1\n")
			sb.WriteString("ieee80211n=1\n")
		} else if e.params.Band == "2.4G" {
			sb.WriteString("ieee80211n=1\n")
		}
	case "ac":
		sb.WriteString("ieee80211ac=1\n")
		sb.WriteString("ieee80211n=1\n")
	case "n":
		sb.WriteString("ieee80211n=1\n")
	}

	// Bandwidth → ht/vht/he/eht capabilities
	if e.params.Band == "6G" || e.params.Band == "6g" {
		if e.params.Bandwidth >= 40 {
			if e.params.Mode == "be" {
				if e.params.Bandwidth == 320 {
					sb.WriteString("eht_oper_chwidth=3\n")
				} else if e.params.Bandwidth == 160 {
					sb.WriteString("eht_oper_chwidth=2\n")
				} else if e.params.Bandwidth == 80 {
					sb.WriteString("eht_oper_chwidth=1\n")
				} else {
					sb.WriteString("eht_oper_chwidth=0\n")
				}
			}
			if e.params.Bandwidth == 160 || e.params.Bandwidth == 320 {
				sb.WriteString("he_oper_chwidth=2\n")
			} else if e.params.Bandwidth == 80 {
				sb.WriteString("he_oper_chwidth=1\n")
			} else {
				sb.WriteString("he_oper_chwidth=0\n")
			}
			centerCh := e.calcHE6GCenterChannel()
			if centerCh > 0 {
				heCenter := centerCh
				if e.params.Bandwidth == 320 {
					// 320MHz has two 160MHz sub-bands: (center - 16) and (center + 16).
					// HE only supports up to 160MHz, so he seg0 must point to the 160MHz containing primaryChannel.
					if primaryChannel < centerCh {
						heCenter = centerCh - 16
					} else {
						heCenter = centerCh + 16
					}
				}
				sb.WriteString(fmt.Sprintf("he_oper_centr_freq_seg0_idx=%d\n", heCenter))
				if e.params.Mode == "be" {
					sb.WriteString(fmt.Sprintf("eht_oper_centr_freq_seg0_idx=%d\n", centerCh))
				}
			}
		}
		// op_class is required for 6GHz — hostapd uses it to distinguish 6GHz from 5GHz (both hw_mode=a)
		if e.params.Bandwidth == 320 {
			sb.WriteString("op_class=137\n") // 320 MHz
		} else if e.params.Bandwidth == 160 {
			sb.WriteString("op_class=134\n") // 160 MHz
		} else if e.params.Bandwidth >= 80 {
			sb.WriteString("op_class=133\n") // 80 MHz
		} else if e.params.Bandwidth == 40 {
			sb.WriteString("op_class=132\n") // 40 MHz
		} else {
			sb.WriteString("op_class=131\n") // 20 MHz
		}
		sb.WriteString("wmm_enabled=1\n")
	} else if e.params.Band != "sub-GHz" {
		// Prefer ht_capab from XML FixedParams if available
		xmlHtCapab := ""
		for _, p := range e.module.APConfig.FixedParams {
			if p.Key == "ht_capab" {
				xmlHtCapab = p.Value
				break
			}
		}

		if xmlHtCapab != "" {
			sb.WriteString(fmt.Sprintf("ht_capab=%s\n", xmlHtCapab))
		} else {
			switch {
			case e.params.Bandwidth >= 40:
				htMode := e.calcHT40Mode()
				sb.WriteString(fmt.Sprintf("ht_capab=[%s][SHORT-GI-20][SHORT-GI-40]\n", htMode))
			default:
				sb.WriteString("ht_capab=[HT20][SHORT-GI-20]\n")
			}
		}

		if e.params.Bandwidth >= 80 && (e.params.Mode == "ac" || e.params.Mode == "ax" || e.params.Mode == "be") {
			if e.params.Bandwidth == 160 || e.params.Bandwidth == 320 {
				sb.WriteString("vht_oper_chwidth=2\n")
			} else {
				sb.WriteString("vht_oper_chwidth=1\n")
			}
			if e.params.Mode == "ax" || e.params.Mode == "be" {
				if e.params.Bandwidth == 160 || e.params.Bandwidth == 320 {
					sb.WriteString("he_oper_chwidth=2\n")
				} else {
					sb.WriteString("he_oper_chwidth=1\n")
				}
			}
			if e.params.Mode == "be" {
				if e.params.Bandwidth == 320 {
					sb.WriteString("eht_oper_chwidth=3\n")
				} else if e.params.Bandwidth == 160 {
					sb.WriteString("eht_oper_chwidth=2\n")
				} else {
					sb.WriteString("eht_oper_chwidth=1\n")
				}
			}
			// Calculate center frequency segment
			centerCh := e.calcVHTCenterChannel()
			if centerCh > 0 {
				sb.WriteString(fmt.Sprintf("vht_oper_centr_freq_seg0_idx=%d\n", centerCh))
				if e.params.Mode == "ax" || e.params.Mode == "be" {
					sb.WriteString(fmt.Sprintf("he_oper_centr_freq_seg0_idx=%d\n", centerCh))
				}
				if e.params.Mode == "be" {
					sb.WriteString(fmt.Sprintf("eht_oper_centr_freq_seg0_idx=%d\n", centerCh))
				}
			}
		}

		sb.WriteString("wmm_enabled=1\n")
	}

	// Security
	sb.WriteString("\n# Security\n")
	switch e.params.Security {
	case "WPA2":
		sb.WriteString("wpa=2\n")
		sb.WriteString(fmt.Sprintf("wpa_passphrase=%s\n", e.params.Password))
		sb.WriteString("wpa_key_mgmt=WPA-PSK\n")
		sb.WriteString("rsn_pairwise=CCMP\n")
	case "WPA3":
		sb.WriteString("wpa=2\n")
		if e.module.ID == "quectel_fme170q_865" {
			sb.WriteString(fmt.Sprintf("wpa_passphrase=%s\n", e.params.Password))
		} else {
			sb.WriteString(fmt.Sprintf("sae_password=%s\n", e.params.Password))
		}
		sb.WriteString("wpa_key_mgmt=SAE\n")
		sb.WriteString("rsn_pairwise=CCMP\n")
		sb.WriteString("ieee80211w=2\n")
	case "Open":
		// No security config needed
	}

	// Fixed params from XML
	if len(e.module.APConfig.FixedParams) > 0 {
		sb.WriteString("\n# Fixed parameters\n")
		for _, p := range e.module.APConfig.FixedParams {
			// Skip params already set by generateHostapd logic
			skipKeys := map[string]bool{
				"ht_capab": true, "wmm_enabled": true, "country_code": true,
				"vht_oper_chwidth": true, "vht_oper_centr_freq_seg0_idx": true,
				"he_oper_chwidth": true, "he_oper_centr_freq_seg0_idx": true,
				"eht_oper_chwidth": true, "eht_oper_centr_freq_seg0_idx": true,
				"wpa_pairwise": true, // rsn_pairwise is already set in security block
				"ctrl_interface": true, "ctrl_interface_group": true,
			}
			if e.params.Band != "sub-GHz" {
				skipKeys["op_class"] = true
			}
			if e.params.Security == "WPA3" {
				skipKeys["ieee80211w"] = true // already set in security block
			}
			if skipKeys[p.Key] {
				continue
			}
			// Skip SAE (WPA3) parameters and mandatory PMF if security is not WPA3
			if e.params.Security != "WPA3" {
				if strings.HasPrefix(p.Key, "sae_") || p.Key == "ieee80211w" {
					continue
				}
			}
			// Skip HE (WiFi 6) parameters if mode is not ax/be
			if e.params.Mode != "ax" && e.params.Mode != "be" && strings.HasPrefix(p.Key, "he_") {
				continue
			}
			// Skip EHT (WiFi 7) parameters if mode is not be
			if e.params.Mode != "be" && strings.HasPrefix(p.Key, "eht_") {
				continue
			}
			// Skip 6GHz ACS parameters if not in 6G band
			if e.params.Band != "6G" && strings.HasPrefix(p.Key, "acs_exclude_6ghz") {
				continue
			}
			sb.WriteString(fmt.Sprintf("%s=%s\n", p.Key, p.Value))
		}
	}

	return sb.String()
}

// generateDnsmasq generates dnsmasq.conf content from current params
func (e *APConfigEditor) generateDnsmasq() string {
	var sb strings.Builder

	sb.WriteString("# dnsmasq configuration for AP mode\n")
	sb.WriteString(fmt.Sprintf("# Module: %s\n", e.module.DisplayName))
	sb.WriteString("# Generated by QWiFiTestTool\n\n")

	sb.WriteString("user=root\n")

	// Helper to get dnsmasq params from both FixedParams and Params
	getDnsmasqParam := func(key string) string {
		for _, p := range e.module.APConfig.FixedParams {
			if p.Key == key {
				return p.Value
			}
		}
		for _, p := range e.module.APConfig.Params {
			if p.Key == key {
				return p.Value
			}
		}
		return ""
	}

	// Check if bind_interfaces is explicitly disabled
	bindInterfaces := true
	if getDnsmasqParam("dnsmasq_bind_interfaces") == "0" {
		bindInterfaces = false
	}

	if bindInterfaces {
		iface := e.module.Kernel.GetInterface()
		sb.WriteString(fmt.Sprintf("interface=%s\n", iface))
	}

	if net := e.module.APConfig.Network; net != nil && net.Gateway != "" {
		sb.WriteString(fmt.Sprintf("listen-address=%s\n", net.Gateway))
	}

	if bindInterfaces {
		sb.WriteString("bind-interfaces\n")
	}

	// Network config from XML or defaults
	gateway := "192.168.50.1"
	netmask := "255.255.255.0"
	dhcpStart := "192.168.50.100"
	dhcpEnd := "192.168.50.200"

	if net := e.module.APConfig.Network; net != nil {
		if net.Gateway != "" {
			gateway = net.Gateway
		}
		if net.Netmask != "" {
			netmask = net.Netmask
		}
		if net.DHCPStart != "" {
			dhcpStart = net.DHCPStart
		}
		if net.DHCPEnd != "" {
			dhcpEnd = net.DHCPEnd
		}
	}

	sb.WriteString("\n# DHCP settings\n")
	sb.WriteString(fmt.Sprintf("dhcp-range=%s,%s,12h\n", dhcpStart, dhcpEnd))

	// Check custom dnsmasq_server in XML
	dnsmasqServer := getDnsmasqParam("dnsmasq_server")

	if dnsmasqServer != "" {
		sb.WriteString(fmt.Sprintf("server=%s\n", dnsmasqServer))
	} else {
		// Only write default router/netmask options and 8.8.8.8 if no custom server logic is defined
		sb.WriteString(fmt.Sprintf("dhcp-option=option:router,%s\n", gateway))
		sb.WriteString(fmt.Sprintf("dhcp-option=option:netmask,%s\n", netmask))
		sb.WriteString("dhcp-option=option:dns-server,8.8.8.8,8.8.4.4\n")
		sb.WriteString("\n# DNS settings\n")
		sb.WriteString("server=8.8.8.8\n")
		sb.WriteString("server=8.8.4.4\n")

		sb.WriteString("\n# Logging\n")
		sb.WriteString("log-queries\n")
		sb.WriteString("log-dhcp\n")
	}

	return sb.String()
}

// calcVHTCenterChannel calculates VHT/HE center frequency channel for 80/160MHz
func (e *APConfigEditor) calcVHTCenterChannel() int {
	ch := e.params.Channel
	if e.params.Bandwidth == 160 {
		if ch >= 36 && ch <= 64 {
			return 50
		} else if ch >= 100 && ch <= 128 {
			return 114
		} else if ch >= 149 && ch <= 177 {
			return 163
		}
	} else {
		// 80MHz channel groups and their center channels
		groups := [][2]int{
			{36, 42}, {52, 58}, {100, 106}, {116, 122},
			{132, 138}, {149, 155},
		}
		for _, g := range groups {
			start := g[0]
			center := g[1]
			if ch >= start && ch < start+16 {
				return center
			}
		}
	}
	return 0
}

// calcHE6GCenterChannel calculates HE/EHT center frequency channel in 6GHz
func (e *APConfigEditor) calcHE6GCenterChannel() int {
	ch := e.params.Channel
	if e.params.Bandwidth > 20 && (ch-1)%4 != 0 {
		return ch
	}
	if e.params.Bandwidth == 320 {
		centers := []int{31, 63, 95, 127, 159, 191}
		for _, c := range centers {
			if ch >= c-30 && ch <= c+30 {
				return c
			}
		}
	} else if e.params.Bandwidth == 160 {
		centers := []int{15, 47, 79, 111, 143, 175, 207}
		for _, c := range centers {
			if ch >= c-14 && ch <= c+14 {
				return c
			}
		}
	} else if e.params.Bandwidth == 80 {
		// 80MHz center channels in 6GHz: 7, 23, 39, 55, 71, 87, 103, 119, 135, 151, 167, 183, 199, 215, 231
		centers := []int{7, 23, 39, 55, 71, 87, 103, 119, 135, 151, 167, 183, 199, 215, 231}
		for _, c := range centers {
			if ch >= c-6 && ch <= c+6 {
				return c
			}
		}
	} else if e.params.Bandwidth == 40 {
		// 40MHz center channels
		centers := []int{3, 11, 19, 27, 35, 43, 51, 59, 67, 75, 83, 91, 99, 107, 115, 123, 131, 139, 147, 155, 163, 171, 179, 187, 195, 203, 211, 219, 227}
		for _, c := range centers {
			if ch >= c-2 && ch <= c+2 {
				return c
			}
		}
	}
	return 0
}

// CalcPrimaryChannel6G calculates the recommended 20MHz primary PSC channel for a 6GHz center channel
func CalcPrimaryChannel6G(centerCh int, bw int) int {
	if bw <= 20 || (centerCh-1)%4 == 0 {
		return centerCh
	}
	switch bw {
	case 40:
		if (centerCh+2-1)%16 == 4 {
			return centerCh + 2
		}
		return centerCh - 2
	case 80:
		// For any 80MHz center C, C-2 is always the PSC channel (5, 21, 37, 53, 69, 85...)
		return centerCh - 2
	case 160:
		// 160MHz center C: lower 80M has PSC at C-10 (e.g. 15 -> 5, 47 -> 37, 79 -> 69)
		return centerCh - 10
	case 320:
		// 320MHz center C: C+6 is always a PSC (e.g. 31 -> 37, 95 -> 101, 159 -> 165)
		return centerCh + 6
	default:
		return centerCh - 2
	}
}

func (e *APConfigEditor) readLine() string {
	input, _ := e.reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// calcHT40Mode determines HT40+ or HT40- based on channel number
// 2.4GHz: channels 1-7 → HT40+, channels 8-13 → HT40-
// 5GHz:   36,44,52,60,100,108,116,124,132,149,157 → HT40+, others → HT40-
func (e *APConfigEditor) calcHT40Mode() string {
	ch := e.params.Channel

	if e.params.Band == "2.4G" {
		if ch <= 7 {
			return "HT40+"
		}
		return "HT40-"
	}

	// 5GHz: first channel of each 40MHz pair uses HT40+
	ht40PlusChannels := map[int]bool{
		36: true, 44: true, 52: true, 60: true,
		100: true, 108: true, 116: true, 124: true,
		132: true, 140: true, 149: true, 157: true,
	}
	if ht40PlusChannels[ch] {
		return "HT40+"
	}
	return "HT40-"
}

// loadProfileTemplate shows a menu of embedded hostapd profiles and loads the selected one.
// Returns (true, nil) if a profile was loaded, (false, nil) if cancelled, or (false, err) on error.
func (e *APConfigEditor) loadProfileTemplate() (bool, error) {
	profiles, err := GetAvailableProfiles()
	if err != nil {
		return false, err
	}
	if len(profiles) == 0 {
		return false, fmt.Errorf(i18n.T("ap_ed.tpl.none"))
	}

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("ap_ed.tpl.title") + " ==========" + ui.Reset)
	for i, p := range profiles {
		fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %s\n", i+1, p.Name)
	}
	fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " " + i18n.T("ap_ed.tpl.cancel"))
	fmt.Print(ui.Yellow + ui.Bold + i18n.T("ap_ed.tpl.select") + ui.Reset)

	input := e.readLine()
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 0 || idx > len(profiles) {
		return false, fmt.Errorf(i18n.T("ap_ed.tpl.invalid"))
	}
	if idx == 0 {
		return false, nil // cancelled, not an error
	}

	selected := profiles[idx-1]
	e.applyProfileParams(selected.ParsedParams)
	fmt.Println(i18n.Tf("ap_ed.tpl.loaded", selected.Name))
	return true, nil
}

func (e *APConfigEditor) applyProfileParams(configMap map[string]string) {
	if v, ok := configMap["ssid"]; ok {
		e.params.SSID = v
	}
	if v, ok := configMap["country_code"]; ok {
		e.params.CountryCode = v
	}
	if v, ok := configMap["channel"]; ok {
		if ch, err := strconv.Atoi(v); err == nil {
			e.params.Channel = ch
		}
	}

	// hw_mode → Band
	if v, ok := configMap["hw_mode"]; ok {
		if configMap["op_class"] == "131" || configMap["op_class"] == "132" || configMap["op_class"] == "133" || configMap["op_class"] == "134" || configMap["op_class"] == "135" || configMap["op_class"] == "136" || configMap["op_class"] == "137" || (v == "a" && configMap["channel"] == "37") {
			e.params.Band = "6G"
		} else if v == "a" {
			e.params.Band = "5G"
		} else {
			e.params.Band = "2.4G"
		}
	}

	// ieee80211 flags → Mode
	if configMap["ieee80211ah"] == "1" {
		e.params.Mode = "ah"
		e.params.Band = "sub-GHz"
	} else if configMap["ieee80211be"] == "1" {
		e.params.Mode = "be"
	} else if configMap["ieee80211ax"] == "1" {
		e.params.Mode = "ax"
	} else if configMap["ieee80211ac"] == "1" {
		e.params.Mode = "ac"
	} else if configMap["ieee80211n"] == "1" {
		e.params.Mode = "n"
	} else {
		// Pure b/g/a without ieee80211n/ac/ax flags — treat as legacy
		// Keep existing mode as-is; generateHostapd will emit hw_mode only
	}

	// ht_capab → Bandwidth
	if ht, ok := configMap["ht_capab"]; ok {
		if strings.Contains(ht, "HT40") {
			e.params.Bandwidth = 40
		} else {
			e.params.Bandwidth = 20
		}
	} else {
		e.params.Bandwidth = 20 // default
	}

	if configMap["vht_oper_chwidth"] == "1" || configMap["he_oper_chwidth"] == "1" || configMap["eht_oper_chwidth"] == "1" {
		e.params.Bandwidth = 80
	}
	if configMap["vht_oper_chwidth"] == "2" || configMap["he_oper_chwidth"] == "2" || configMap["eht_oper_chwidth"] == "2" {
		e.params.Bandwidth = 160
	}
	if configMap["eht_oper_chwidth"] == "3" {
		e.params.Bandwidth = 320
	}
	// In 6GHz, bandwidth is directly indicated by op_class
	if op, ok := configMap["op_class"]; ok {
		switch op {
		case "131":
			e.params.Bandwidth = 20
		case "132":
			e.params.Bandwidth = 40
		case "133":
			e.params.Bandwidth = 80
		case "134":
			e.params.Bandwidth = 160
		case "135": // 80+80 MHz (deprecated in WiFi 7, treat as 160)
			e.params.Bandwidth = 160
		case "136": // 20 MHz (6GHz discovery channel)
			e.params.Bandwidth = 20
		case "137":
			e.params.Bandwidth = 320
		}
	}

	// Security from profile
	if configMap["wpa"] == "2" {
		if configMap["wpa_key_mgmt"] == "SAE" {
			e.params.Security = "WPA3"
		} else {
			e.params.Security = "WPA2"
		}
		if v, ok := configMap["sae_password"]; ok {
			e.params.Password = v
		} else if v, ok := configMap["wpa_passphrase"]; ok {
			e.params.Password = v
		}
	} else {
		e.params.Security = "Open"
		e.params.Password = ""
	}

	// Managed keys are handled by high-level params (e.params) or generated
	// explicitly by generateHostapd(). We skip these to avoid duplicates.
	managedKeys := map[string]bool{
		"ssid": true, "channel": true, "hw_mode": true, "country_code": true,
		"ieee80211n": true, "ieee80211ac": true, "ieee80211ax": true, "ieee80211ah": true,
		"ieee80211be": true, "ieee80211d": true,
		"wpa":        true, "wpa_key_mgmt": true, "sae_password": true, "wpa_passphrase": true,
		"ieee80211w": true, "rsn_pairwise": true,
		"vht_oper_chwidth": true, "he_oper_chwidth": true, "eht_oper_chwidth": true,
		"vht_oper_centr_freq_seg0_idx": true, "he_oper_centr_freq_seg0_idx": true, "eht_oper_centr_freq_seg0_idx": true,
		"wmm_enabled": true,
		"interface":   true, "driver": true,
		// daemon-level params that should not leak into generated config
		"ctrl_interface": true, "ctrl_interface_group": true,
	}
	if e.params.Band != "sub-GHz" {
		managedKeys["op_class"] = true
	}

	// Build a set of keys coming from the new profile, so we can evict
	// stale entries left over from a *previous* profile load.
	profileKeys := make(map[string]bool, len(configMap))
	for k := range configMap {
		profileKeys[k] = true
	}

	var newFixed []models.Param
	// 1. Keep existing FixedParams that are:
	//    - NOT managed by UI/generator
	//    - NOT about to be replaced by this new profile's keys
	if e.module != nil {
		for _, p := range e.module.APConfig.FixedParams {
			if managedKeys[p.Key] {
				continue // already generated by code
			}
			if profileKeys[p.Key] {
				continue // will be replaced by the new profile value below
			}
			newFixed = append(newFixed, p)
		}
	}

	// 2. Add non-managed parameters from the profile
	for k, v := range configMap {
		if !managedKeys[k] {
			newFixed = append(newFixed, models.Param{Key: k, Value: v})
		}
	}

	if e.module != nil {
		e.module.APConfig.FixedParams = newFixed
	}
}

// isValidPSCForBlock checks if the user's channel is already a valid PSC channel within the specified block
func isValidPSCForBlock(ch int, centerCh int, bw int) bool {
	// A valid 6GHz PSC channel is 5, 21, 37, 53, 69, 85... -> (ch-5)%16 == 0
	if (ch-5)%16 != 0 {
		return false
	}
	
	var maxDist int
	switch bw {
	case 40:
		maxDist = 2
	case 80:
		maxDist = 6
	case 160:
		maxDist = 14
	case 320:
		maxDist = 30
	default:
		maxDist = 0
	}
	return ch >= centerCh-maxDist && ch <= centerCh+maxDist
}

// isDFSChannel checks if a given 5GHz operating block intersects with DFS channels (52-144)
func isDFSChannel(band string, ch int, centerCh int, bw int) bool {
	bandUpper := strings.ToUpper(band)
	if bandUpper != "5G" {
		return false
	}
	// Direct DFS channels in 5GHz: UNII-2A (52-64) and UNII-2C (100-144)
	if (ch >= 52 && ch <= 64) || (ch >= 100 && ch <= 144) {
		return true
	}
	// For 160MHz, channels 36-48 bond UNII-1 with UNII-2A (channels 52-64)
	if bw == 160 && (ch >= 36 && ch <= 48) {
		return true
	}
	// UNII-3 (ch 149-177) is NOT DFS — exclude before fallback range check
	if ch >= 149 && ch <= 177 {
		return false
	}
	// Fallback to center channel range check if centerCh is provided
	if centerCh > 0 && bw >= 80 {
		var maxDist int
		if bw == 160 {
			maxDist = 14
		} else {
			maxDist = 6
		}
		minCh := centerCh - maxDist
		maxCh := centerCh + maxDist
		return (maxCh >= 52 && minCh <= 64) || (maxCh >= 100 && minCh <= 144)
	}
	return false
}
