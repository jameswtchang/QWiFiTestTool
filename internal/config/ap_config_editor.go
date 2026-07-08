package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

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
		return nil, fmt.Errorf("載入國家頻道資料庫失敗: %w", err)
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
		APType:      "室内型AP",
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
	fmt.Println(ui.Cyan + ui.Bold + "║     Hostapd / Dnsmasq 配置生成及修改              ║" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "╚═══════════════════════════════════════════════════╝" + ui.Reset)
	fmt.Printf("[模組: %s]\n", e.module.DisplayName)

	// Check if system config exists
	hostapdPath := "/etc/hostapd.conf"
	if fileExists(hostapdPath) {
		fmt.Printf("\n"+ui.Cyan+"[INFO] "+ui.Reset+"偵測到系統配置: %s\n", hostapdPath)
		e.parseExistingHostapd(hostapdPath)
	} else {
		fmt.Println("\n" + ui.Cyan + "[INFO] " + ui.Reset + "系統配置不存在，使用 XML 預設值")
	}

	for {
		e.showCurrentConfig()

		fmt.Println("\n" + ui.Cyan + ui.Bold + "[1]" + ui.Reset + " 修改當前配置")
		fmt.Println(ui.Cyan + ui.Bold + "[2]" + ui.Reset + " 捨棄舊設定，重新生成 (使用 XML 預設值)")
		fmt.Println(ui.Cyan + ui.Bold + "[3]" + ui.Reset + " 從預設情境範本載入 (快速套用 11n/ac/ax 等情境)")
		fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " 退出不修改")
		fmt.Print(ui.Yellow + ui.Bold + "選擇操作: " + ui.Reset)

		input := e.readLine()

		switch input {
		case "1":
			e.editParams()
			saved, err := e.generateAndPreview()
			if err != nil {
				fmt.Printf("\n❌ 錯誤: %v\n", err)
			} else if saved {
				return nil
			}
		case "2":
			e.loadDefaults()
			fmt.Println("[INFO] 已恢復此模組的預設值")
			saved, err := e.generateAndPreview()
			if err != nil {
				fmt.Printf("\n❌ 錯誤: %v\n", err)
			} else if saved {
				return nil
			}
		case "3":
			loaded, err := e.loadProfileTemplate()
			if err != nil {
				fmt.Printf("\n❌ 載入範本失敗: %v\n", err)
			} else if loaded {
				saved, err := e.generateAndPreview()
				if err != nil {
					fmt.Printf("\n❌ 錯誤: %v\n", err)
				} else if saved {
					return nil
				}
			}
		case "0":
			return nil
		default:
			fmt.Println("❌ 無效選項")
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
		if configMap["op_class"] == "131" || configMap["op_class"] == "132" || configMap["op_class"] == "133" || configMap["op_class"] == "134" || configMap["op_class"] == "135" || configMap["op_class"] == "136" || configMap["op_class"] == "137" || configMap["acs_exclude_6ghz_non_psc"] == "1" || (v == "a" && configMap["channel"] == "37") {
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
	} else if configMap["ieee80211ax"] == "1" {
		e.params.Mode = "ax"
	} else if configMap["ieee80211ac"] == "1" {
		e.params.Mode = "ac"
	} else if configMap["ieee80211n"] == "1" {
		e.params.Mode = "n"
	}

	// ht_capab → Bandwidth
	if ht, ok := configMap["ht_capab"]; ok {
		if strings.Contains(ht, "HT40") {
			e.params.Bandwidth = 40
		} else {
			e.params.Bandwidth = 20
		}
	}
	if configMap["vht_oper_chwidth"] == "1" {
		e.params.Bandwidth = 80
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
}

// showCurrentConfig displays the current parameter state
func (e *APConfigEditor) showCurrentConfig() {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== 當前配置 (Hostapd) ==========" + ui.Reset)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("國家碼:", 14), e.params.CountryCode)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("AP類型:", 14), e.params.APType)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("頻段:", 14), e.params.Band)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("協議:", 14), e.modeDisplayName(e.params.Mode))
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%dMHz"+ui.Reset+"\n", ui.AlignLabel("頻寬:", 14), e.params.Bandwidth)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%d"+ui.Reset+"\n", ui.AlignLabel("頻道:", 14), e.params.Channel)
	fmt.Printf(ui.Bold+"  %s "+ui.Cyan+"%s"+ui.Reset+"\n", ui.AlignLabel("SSID:", 14), e.params.SSID)
	fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel("安全性:", 14), e.params.Security)
	if e.params.Security != "Open" {
		fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel("密碼:", 14), e.params.Password)
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

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== 當前配置 (Dnsmasq) ==========" + ui.Reset)
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
		fmt.Println("\n" + ui.Yellow + ui.Bold + "---------- 修改參數 ----------" + ui.Reset)
		fmt.Println(ui.Cyan + ui.Bold + "[1]" + ui.Reset + " 國家碼")
		fmt.Println(ui.Cyan + ui.Bold + "[2]" + ui.Reset + " AP類型")
		fmt.Println(ui.Cyan + ui.Bold + "[3]" + ui.Reset + " 頻段")
		fmt.Println(ui.Cyan + ui.Bold + "[4]" + ui.Reset + " 協議")
		fmt.Println(ui.Cyan + ui.Bold + "[5]" + ui.Reset + " 頻寬")
		fmt.Println(ui.Cyan + ui.Bold + "[6]" + ui.Reset + " 頻道")
		fmt.Println(ui.Cyan + ui.Bold + "[7]" + ui.Reset + " SSID")
		fmt.Println(ui.Cyan + ui.Bold + "[8]" + ui.Reset + " 安全性")
		fmt.Println(ui.Cyan + ui.Bold + "[9]" + ui.Reset + " 密碼")
		fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " 完成修改")
		fmt.Print(ui.Yellow + ui.Bold + "選擇要修改的項目: " + ui.Reset)

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
			fmt.Println("❌ 無效選項")
		}
	}
}

// --- Parameter editors ---

func (e *APConfigEditor) editCountryCode() {
	codes := e.channelDB.ListCountryCodes()
	fmt.Printf("\n可用國碼 (%d 個):\n", len(codes))

	// Show in columns
	cols := 10
	for i, code := range codes {
		fmt.Printf("%-5s", code)
		if (i+1)%cols == 0 {
			fmt.Println()
		}
	}
	fmt.Println()

	fmt.Printf("輸入國碼 [當前: %s]: ", e.params.CountryCode)
	input := strings.ToUpper(e.readLine())
	if input == "" {
		return
	}

	if e.channelDB.FindCountry(input) != nil {
		e.params.CountryCode = input
		e.refreshChannels()
		fmt.Printf("✓ 國碼已設為 %s\n", input)
	} else {
		fmt.Println("❌ 無效的國碼")
	}
}

func (e *APConfigEditor) editAPType() {
	country := e.channelDB.FindCountry(e.params.CountryCode)
	if country == nil {
		fmt.Println("❌ 請先選擇有效的國碼")
		return
	}

	types := country.GetAPTypes()
	fmt.Println("\n可用 AP 類型:")
	for i, t := range types {
		marker := "  "
		if t == e.params.APType {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %s\n", marker, i+1, t)
	}

	fmt.Print("選擇（輸入編號）: ")
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(types) {
		fmt.Println("❌ 無效選擇")
		return
	}

	e.params.APType = types[idx-1]
	e.refreshChannels()
	fmt.Printf("✓ AP 類型已設為 %s\n", e.params.APType)
}

func (e *APConfigEditor) editBand() {
	caps := e.module.APConfig.Capabilities
	bands := []string{"2.4G"}
	if caps != nil {
		bands = caps.GetBands()
	}

	fmt.Println("\n可用頻段:")
	for i, b := range bands {
		marker := "  "
		if strings.TrimSpace(b) == e.params.Band {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %s\n", marker, i+1, strings.TrimSpace(b))
	}

	if len(bands) == 1 {
		fmt.Println("[INFO] 此模組僅支援一個頻段")
		e.params.Band = strings.TrimSpace(bands[0])
		return
	}

	fmt.Print("選擇（輸入編號）: ")
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(bands) {
		fmt.Println("❌ 無效選擇")
		return
	}

	e.params.Band = strings.TrimSpace(bands[idx-1])
	e.refreshChannels()
	fmt.Printf("✓ 頻段已設為 %s\n", e.params.Band)
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
	}

	fmt.Println("\n可用協議:")
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

	fmt.Print("選擇（輸入編號）: ")
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(available) {
		fmt.Println("❌ 無效選擇")
		return
	}

	e.params.Mode = available[idx-1].id
	fmt.Printf("✓ 協議已設為 %s\n", available[idx-1].display)
}

func (e *APConfigEditor) editBandwidth() {
	caps := e.module.APConfig.Capabilities
	maxBW := 160
	if caps != nil && caps.MaxBandwidth > 0 {
		maxBW = caps.MaxBandwidth
	}

	allBW := []int{20, 40, 80, 160}
	fmt.Println("\n可用頻寬:")
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

	fmt.Print("選擇（輸入編號）: ")
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(available) {
		fmt.Println("❌ 無效選擇")
		return
	}

	e.params.Bandwidth = available[idx-1]
	e.refreshChannels()
	fmt.Printf("✓ 頻寬已設為 %dMHz\n", available[idx-1])
}

func (e *APConfigEditor) editChannel() {
	channels := e.channelDB.LookupChannels(
		e.params.CountryCode, e.params.APType, e.params.Band, e.params.Bandwidth,
	)

	if len(channels) == 0 {
		fmt.Println("❌ 目前的 國碼/AP類型/頻段/頻寬 組合無可用頻道")
		return
	}

	fmt.Println("\n可用頻道:")
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

	fmt.Printf("輸入頻道號碼 [當前: %d]: ", e.params.Channel)
	input := e.readLine()
	if input == "" {
		return
	}

	ch, err := strconv.Atoi(input)
	if err != nil {
		fmt.Println("❌ 無效的頻道")
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
		fmt.Println("⚠ 此頻道不在可用清單中，仍然設定")
	}

	e.params.Channel = ch
	fmt.Printf("✓ 頻道已設為 %d\n", ch)
}

func (e *APConfigEditor) editSSID() {
	fmt.Printf("輸入 SSID [當前: %s]: ", e.params.SSID)
	input := e.readLine()
	if input != "" {
		e.params.SSID = input
		fmt.Printf("✓ SSID 已設為 %s\n", input)
	}
}

func (e *APConfigEditor) editSecurity() {
	options := []string{"Open", "WPA2"}
	caps := e.module.APConfig.Capabilities
	if caps != nil && caps.SupportWPA3 {
		options = append(options, "WPA3")
	}

	fmt.Println("\n安全性選項:")
	for i, opt := range options {
		marker := "  "
		if opt == e.params.Security {
			marker = "→ "
		}
		fmt.Printf("%s[%d] %s\n", marker, i+1, opt)
	}

	fmt.Print("選擇（輸入編號）: ")
	idx, err := strconv.Atoi(e.readLine())
	if err != nil || idx < 1 || idx > len(options) {
		fmt.Println("❌ 無效選擇")
		return
	}

	e.params.Security = options[idx-1]
	if e.params.Security == "Open" {
		e.params.Password = ""
	}
	fmt.Printf("✓ 安全性已設為 %s\n", options[idx-1])
}

func (e *APConfigEditor) editPassword() {
	if e.params.Security == "Open" {
		fmt.Println("[INFO] 安全性為 Open 時無需密碼")
		return
	}

	for {
		fmt.Printf("輸入密碼 [當前: %s]: ", e.params.Password)
		input := e.readLine()

		if input == "" {
			break
		}

		if len(input) < 8 || len(input) > 63 {
			fmt.Println("❌ 錯誤: WPA 密碼長度必須介於 8 到 63 個字元之間")
			continue
		}

		e.params.Password = input
		fmt.Printf("✓ 密碼已更新\n")
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
			fmt.Printf("[INFO] 頻道已自動調整為 %d\n", e.params.Channel)
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
	fmt.Println(ui.Cyan + ui.Bold + "║              hostapd.conf 預覽                    ║" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "╚═══════════════════════════════════════════════════╝" + ui.Reset)
	fmt.Println(ui.Green + hostapdContent + ui.Reset)

	fmt.Println(ui.Cyan + ui.Bold + "╔═══════════════════════════════════════════════════╗" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "║              dnsmasq.conf 預覽                    ║" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "╚═══════════════════════════════════════════════════╝" + ui.Reset)
	fmt.Println(ui.Green + dnsmasqContent + ui.Reset)

	// Check if files already exist
	hostapdExists := fileExists(hostapdPath)
	dnsmasqExists := fileExists(dnsmasqPath)

	if hostapdExists || dnsmasqExists {
		fmt.Println("\n" + ui.Yellow + ui.Bold + "⚠ 注意: 系統配置檔已存在，寫入將會覆蓋舊有設定:" + ui.Reset)
		if hostapdExists {
			fmt.Printf("  - %s\n", hostapdPath)
		}
		if dnsmasqExists {
			fmt.Printf("  - %s\n", dnsmasqPath)
		}
	}

	// Prompt to save
	fmt.Printf("\n" + ui.Yellow + ui.Bold + "確認寫入系統配置? [y/N]: " + ui.Reset)
	if response := e.readLine(); response == "y" || response == "Y" {
		if err := os.WriteFile(hostapdPath, []byte(hostapdContent), 0644); err != nil {
			return false, fmt.Errorf("寫入 %s 失敗: %w", hostapdPath, err)
		}
		if err := os.WriteFile(dnsmasqPath, []byte(dnsmasqContent), 0644); err != nil {
			return false, fmt.Errorf("寫入 %s 失敗: %w", dnsmasqPath, err)
		}
		fmt.Println("\n" + ui.Green + ui.Bold + "✓ 已儲存:" + ui.Reset)
		fmt.Printf("  - %s\n", hostapdPath)
		fmt.Printf("  - %s\n", dnsmasqPath)
		fmt.Println(ui.Cyan + "[INFO] " + ui.Reset + "AP 模式 (選單 5) 將自動使用此配置\n")
		
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

	// SSID
	sb.WriteString(fmt.Sprintf("ssid=%s\n", e.params.SSID))

	// Country — skip if driver handles regulatory via rtw_country_code
	if !strings.Contains(e.module.Kernel.InsmodArgs, "rtw_country_code") {
		sb.WriteString(fmt.Sprintf("country_code=%s\n", e.params.CountryCode))
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
	sb.WriteString(fmt.Sprintf("channel=%d\n", e.params.Channel))

	// Mode → ieee80211 flags
	switch e.params.Mode {
	case "ah":
		sb.WriteString("ieee80211ah=1\n")
	case "ax":
		sb.WriteString("ieee80211ax=1\n")
		if e.params.Band != "6G" && e.params.Band != "6g" {
			sb.WriteString("ieee80211ac=1\n")
			sb.WriteString("ieee80211n=1\n")
		}
	case "ac":
		sb.WriteString("ieee80211ac=1\n")
		sb.WriteString("ieee80211n=1\n")
	case "n":
		sb.WriteString("ieee80211n=1\n")
	}

	// Bandwidth → ht/vht capabilities
	if e.params.Band == "6G" || e.params.Band == "6g" {
		if e.params.Bandwidth >= 80 {
			sb.WriteString("he_oper_chwidth=1\n")
			centerCh := e.calcHE6GCenterChannel()
			if centerCh > 0 {
				sb.WriteString(fmt.Sprintf("he_oper_centr_freq_seg0_idx=%d\n", centerCh))
			}
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

		if e.params.Bandwidth >= 80 && (e.params.Mode == "ac" || e.params.Mode == "ax") {
			sb.WriteString("vht_oper_chwidth=1\n")
			if e.params.Mode == "ax" {
				sb.WriteString("he_oper_chwidth=1\n")
			}
			// Calculate center frequency segment
			centerCh := e.calcVHTCenterChannel()
			if centerCh > 0 {
				sb.WriteString(fmt.Sprintf("vht_oper_centr_freq_seg0_idx=%d\n", centerCh))
				if e.params.Mode == "ax" {
					sb.WriteString(fmt.Sprintf("he_oper_centr_freq_seg0_idx=%d\n", centerCh))
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
		sb.WriteString(fmt.Sprintf("sae_password=%s\n", e.params.Password))
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
			}
			if skipKeys[p.Key] {
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

// calcVHTCenterChannel calculates VHT center frequency channel for 80MHz
func (e *APConfigEditor) calcVHTCenterChannel() int {
	ch := e.params.Channel
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
	return 0
}

// calcHE6GCenterChannel calculates HE center frequency channel for 80MHz in 6GHz
func (e *APConfigEditor) calcHE6GCenterChannel() int {
	ch := e.params.Channel
	// 80MHz center channels in 6GHz: 7, 23, 39, 55, 71, 87, 103, 119, 135, 151, 167, 183, 199, 215, 231
	centers := []int{7, 23, 39, 55, 71, 87, 103, 119, 135, 151, 167, 183, 199, 215, 231}
	for _, c := range centers {
		if ch >= c-6 && ch <= c+6 {
			return c
		}
	}
	return 0
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
		132: true, 149: true, 157: true,
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
		return false, fmt.Errorf("找不到任何情境範本")
	}

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== 選擇情境範本 ==========" + ui.Reset)
	for i, p := range profiles {
		fmt.Printf(ui.Cyan+ui.Bold+"[%d]"+ui.Reset+" %s\n", i+1, p.Name)
	}
	fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " 取消")
	fmt.Print(ui.Yellow + ui.Bold + "選擇範本編號: " + ui.Reset)

	input := e.readLine()
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 0 || idx > len(profiles) {
		return false, fmt.Errorf("無效選擇")
	}
	if idx == 0 {
		return false, nil // cancelled, not an error
	}

	selected := profiles[idx-1]
	e.applyProfileParams(selected.ParsedParams)
	fmt.Printf("\n✓ 成功載入範本: %s\n", selected.Name)
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
		if configMap["op_class"] == "131" || configMap["op_class"] == "132" || configMap["op_class"] == "133" || configMap["op_class"] == "134" || configMap["op_class"] == "135" || configMap["op_class"] == "136" || configMap["op_class"] == "137" || configMap["acs_exclude_6ghz_non_psc"] == "1" || (v == "a" && configMap["channel"] == "37") {
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

	if configMap["vht_oper_chwidth"] == "1" || configMap["he_oper_chwidth"] == "1" {
		e.params.Bandwidth = 80
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
		"ieee80211d": true,
		"wpa": true, "wpa_key_mgmt": true, "sae_password": true, "wpa_passphrase": true,
		"ieee80211w": true, "rsn_pairwise": true,
		"vht_oper_chwidth": true, "he_oper_chwidth": true,
		"vht_oper_centr_freq_seg0_idx": true, "he_oper_centr_freq_seg0_idx": true,
		"wmm_enabled": true,
		"interface": true, "driver": true,
		// daemon-level params that should not leak into generated config
		"ctrl_interface": true, "ctrl_interface_group": true,
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
	for _, p := range e.module.APConfig.FixedParams {
		if managedKeys[p.Key] {
			continue // already generated by code
		}
		if profileKeys[p.Key] {
			continue // will be replaced by the new profile value below
		}
		newFixed = append(newFixed, p)
	}

	// 2. Add non-managed parameters from the profile
	for k, v := range configMap {
		if !managedKeys[k] {
			newFixed = append(newFixed, models.Param{Key: k, Value: v})
		}
	}

	e.module.APConfig.FixedParams = newFixed
}
