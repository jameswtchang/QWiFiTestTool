package wifi

import (

	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"qwifitesttool/internal/config"
	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/logger"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/ui"
)

// Hotspot handles AP mode operations (SoftAP/Hotspot)
type Hotspot struct {
	module        *models.WiFiModule
	hostapdPath   string
	dnsmasqPath   string
	hostapdConfig string
	dnsmasqConfig string
	logDir        string
	hostapdCmd    *exec.Cmd
	dnsmasqCmd    *exec.Cmd
	hostapdLog    *logger.TimestampWriter
	dnsmasqLog    *logger.TimestampWriter
}

// NewHotspot creates a new hotspot instance
func NewHotspot(module *models.WiFiModule, hostapdPath string) *Hotspot {
	return &Hotspot{
		module:      module,
		hostapdPath: hostapdPath,
		dnsmasqPath: "dnsmasq", // Will be resolved dynamically
		logDir:      "/tmp/wifi_logs",
	}
}

// Start initiates the AP mode workflow
func (h *Hotspot) Start() error {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("hotspot.title") + " ==========" + ui.Reset)

	// Setup hostapd config
	if err := h.setupHostapdConfig(); err != nil {
		return err
	}

	// Setup dnsmasq config
	if err := h.setupDnsmasqConfig(); err != nil {
		return err
	}

	// Start services
	return h.startServices()
}

// setupHostapdConfig handles hostapd configuration
func (h *Hotspot) setupHostapdConfig() error {
	confMgr := config.NewConfigManager("hostapd", h.module)

	configPath, exists, err := confMgr.GetConfigPath()
	if err != nil {
		return err
	}
	h.hostapdConfig = configPath

	// Print config file content if exists
	if exists {
		h.printConfigContent("hostapd", configPath)
	} else {
		fmt.Println("[INFO] 找不到系統 AP 配置檔。將引導您進入 AP 配置編輯器...")
		
		editor, err := config.NewAPConfigEditor(h.module)
		if err != nil {
			return fmt.Errorf("初始化 AP 配置編輯器失敗: %w", err)
		}
		if err := editor.Start(); err != nil {
			return fmt.Errorf("AP 配置編輯失敗: %w", err)
		}
		
		// 編輯器完成後，重新取得實際生成的配置路徑
		configPath, exists, err = confMgr.GetConfigPath()
		if err != nil || !exists {
			return fmt.Errorf("未產生有效配置檔，無法啟動 AP")
		}
		h.hostapdConfig = configPath
		
		h.printConfigContent("hostapd", configPath)
	}

	return nil
}

// setupDnsmasqConfig handles dnsmasq configuration
func (h *Hotspot) setupDnsmasqConfig() error {
	confMgr := config.NewConfigManager("dnsmasq", h.module)

	configPath, exists, err := confMgr.GetConfigPath()
	if err != nil {
		return err
	}
	h.dnsmasqConfig = configPath

	// Print config file content if exists
	if exists {
		h.printConfigContent("dnsmasq", configPath)
	} else {
		// Generate default dnsmasq config
		iface := h.module.Kernel.GetInterface()
		cfg := config.DefaultDnsmasqConfig(iface)

		// Apply network overrides from XML
		if h.module.APConfig.Network != nil {
			net := h.module.APConfig.Network
			if net.Gateway != "" {
				cfg.GatewayIP = net.Gateway
			}
			if net.Netmask != "" {
				cfg.SubnetMask = net.Netmask
			}
			if net.DHCPStart != "" && net.DHCPEnd != "" {
				cfg.DHCPRange = fmt.Sprintf("%s,%s,%s", net.DHCPStart, net.DHCPEnd, cfg.LeaseTime)
			}
		}

		configContent := config.GenerateDnsmasqConfig(h.module, cfg)

		if err := config.SaveConfig(h.dnsmasqConfig, configContent); err != nil {
			return err
		}
		fmt.Println(i18n.Tf("hotspot.dnsmasq_gen", h.dnsmasqConfig))

		if confMgr.PromptSaveCustom() {
			confMgr.SaveAsCustomConfig(configContent)
		}
	}

	return nil
}

// printConfigContent reads and prints the config file content
func (h *Hotspot) printConfigContent(name, path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(i18n.Tf("hotspot.config_read_fail", name, err))
		return
	}

	fmt.Printf("\n%s\n", i18n.Tf("hotspot.config_loaded", name, path))
	fmt.Println("----------------------------------------")
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		// Skip empty lines and comments for cleaner output
		if line != "" && !strings.HasPrefix(line, "#") {
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Println("----------------------------------------")
}



// startServices starts hostapd and dnsmasq with logging
func (h *Hotspot) startServices() error {
	// Clean up conflicting daemons and stale interfaces before starting AP
	exec.Command("killall", "-9", "wpa_supplicant").Run()
	exec.Command("killall", "-9", "wpa_supplicant_s1g").Run()
	exec.Command("killall", "-9", "hostapd").Run()
	exec.Command("killall", "-9", "hostapd_s1g").Run()
	exec.Command("killall", "-9", "dnsmasq").Run()
	time.Sleep(300 * time.Millisecond)

	// Create log directory
	if err := os.MkdirAll(h.logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Generate timestamp for log files
	timestamp := time.Now().Format("20060102_150405")
	hostapdLog := filepath.Join(h.logDir, fmt.Sprintf("hostapd_%s.log", timestamp))
	dnsmasqLog := filepath.Join(h.logDir, fmt.Sprintf("dnsmasq_%s.log", timestamp))
	dmesgLog := filepath.Join(h.logDir, fmt.Sprintf("dmesg_ap_%s.log", timestamp))

	// Save dmesg before starting
	saveDmesg(dmesgLog, "before")

	// Morse Micro HaLow specific check: ensure kernel channelization_scheme matches AP config
	if h.module != nil && h.module.ID == "quectel_fgh200m" {
		h.ensureMorseChannelizationScheme()
	}

	// Setup interface IP before starting dnsmasq
	if err := h.setupInterfaceIP(); err != nil {
		fmt.Println(i18n.Tf("hotspot.ip_fail", err))
	}

	// Set regulatory domain before starting hostapd
	h.setRegulatoryDomain()

	// Start hostapd
	if err := h.startHostapd(hostapdLog); err != nil {
		return err
	}

	// Wait up to 6 seconds for hostapd to initialize.
	// PCIe/CNSS based modules (Qualcomm) need extra time for txpower initialization
	// after beacon is set; USB/SDIO modules (Realtek, MTK) may report 0.00 dBm permanently.
	isPCIe := strings.Contains(h.module.Kernel.PreloadKo, "pcie") || strings.Contains(h.module.Kernel.PreloadKo, "cnss")
	apReady := false
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		out, err := exec.Command("iw", "dev", h.module.Kernel.GetInterface(), "info").Output()
		if err != nil {
			continue
		}
		outStr := string(out)
		if !strings.Contains(outStr, "type AP") {
			continue
		}
		// For PCIe modules, also wait until txpower is non-zero
		if isPCIe && strings.Contains(outStr, "txpower 0.00 dBm") {
			continue
		}
		apReady = true
		break
	}

	if !apReady {
		h.Stop()
		return fmt.Errorf("%s", i18n.Tf("hotspot.ap_init_fail", hostapdLog))
	}

	// Start dnsmasq
	if err := h.startDnsmasq(dnsmasqLog); err != nil {
		fmt.Println(i18n.Tf("hotspot.dnsmasq_fail", err))
		// Continue anyway, hostapd can work without dnsmasq
	}

	// Display AP information
	h.showAPInfo()

	fmt.Println("\n" + i18n.T("hotspot.log_files"))
	fmt.Printf("  hostapd: %s\n", hostapdLog)
	fmt.Printf("  dnsmasq: %s\n", dnsmasqLog)
	fmt.Printf("  dmesg:   %s\n", dmesgLog)
	fmt.Println("\n" + i18n.T("hotspot.press_ctrl_c"))

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Wait for signal or process exit
	done := make(chan error, 1)
	go func() {
		if h.hostapdCmd != nil {
			done <- h.hostapdCmd.Wait()
		}
	}()

	select {
	case <-sigChan:
		fmt.Println("\n" + i18n.T("hotspot.stopping"))
	case err := <-done:
		if err != nil {
			fmt.Println(i18n.Tf("hotspot.hostapd_crash", err))
		}
	}

	// Stop all services
	h.Stop()

	// Save dmesg after stopping
	saveDmesg(dmesgLog, "after")

	fmt.Println(i18n.T("hotspot.stopped"))
	return nil
}

// setupInterfaceIP sets the IP address on the wireless interface
func (h *Hotspot) setupInterfaceIP() error {
	iface := h.module.Kernel.GetInterface()

	// Read gateway from XML Network config, fallback to default
	gateway := "192.168.50.1"
	netmask := 24
	if net := h.module.APConfig.Network; net != nil && net.Gateway != "" {
		gateway = net.Gateway
		// Derive prefix length from netmask
		if net.Netmask == "255.255.255.0" {
			netmask = 24
		} else if net.Netmask == "255.255.0.0" {
			netmask = 16
		}
	}
	gatewayIP := fmt.Sprintf("%s/%d", gateway, netmask)

	// Bring interface up
	upCmd := exec.Command("ip", "link", "set", iface, "up")
	cmdStr := fmt.Sprintf("ip link set %s up", iface)
	fmt.Printf("[CMD] %s\n", cmdStr)
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("failed to bring interface up: %w", err)
	}

	// Flush existing IP
	flushCmd := exec.Command("ip", "addr", "flush", "dev", iface)
	cmdStr = fmt.Sprintf("ip addr flush dev %s", iface)
	fmt.Printf("[CMD] %s\n", cmdStr)
	flushCmd.Run() // Ignore error

	// Set IP address
	ipCmd := exec.Command("ip", "addr", "add", gatewayIP, "dev", iface)
	cmdStr = fmt.Sprintf("ip addr add %s dev %s", gatewayIP, iface)
	fmt.Printf("[CMD] %s\n", cmdStr)
	if err := ipCmd.Run(); err != nil {
		return fmt.Errorf("failed to set IP address: %w", err)
	}

	return nil
}

// setRegulatoryDomain sets the wireless regulatory domain via iw reg set
// before hostapd starts, to ensure 5GHz channels are not blocked by NO_IR.
func (h *Hotspot) setRegulatoryDomain() {
	// Realtek drivers handle regulatory domain internally via rtw_country_code
	// Setting iw reg set would conflict (no regdb on most embedded boards)
	if strings.Contains(h.module.Kernel.InsmodArgs, "rtw_country_code") {
		return
	}

	countryCode := ""

	// 1. Try to parse country_code from hostapd.conf
	if h.hostapdConfig != "" {
		if data, err := os.ReadFile(h.hostapdConfig); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "country_code=") {
					countryCode = strings.TrimPrefix(line, "country_code=")
					break
				}
			}
		}
	}

	if countryCode == "" {
		return
	}

	fmt.Println(i18n.Tf("hotspot.set_regdomain", countryCode))
	cmd := exec.Command("iw", "reg", "set", countryCode)
	cmdStr := fmt.Sprintf("iw reg set %s", countryCode)
	fmt.Printf("[CMD] %s\n", cmdStr)
	if err := cmd.Run(); err != nil {
		fmt.Println(i18n.Tf("hotspot.regdomain_fail", err))
	}

	// Give the kernel time to apply the new regulatory domain
	time.Sleep(500 * time.Millisecond)
}

// ensureMorseChannelizationScheme verifies and updates dot11ah channelization_scheme for Morse HaLow AP
func (h *Hotspot) ensureMorseChannelizationScheme() {
	country := "AU"
	if h.hostapdConfig != "" {
		if data, err := os.ReadFile(h.hostapdConfig); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "country_code=") {
					country = strings.TrimPrefix(line, "country_code=")
					break
				}
			}
		}
	}

	targetScheme := "1" // Scheme 1 for AU IEEE802.11-2020 (Op Class 70)
	if country == "US" {
		targetScheme = "3" // Scheme 3 for US (IEEE802.11-REVmf)
	}

	curSchemeBytes, err := os.ReadFile("/sys/module/dot11ah/parameters/channelization_scheme")
	curScheme := ""
	if err == nil {
		curScheme = strings.TrimSpace(string(curSchemeBytes))
	}

	morseLoaded := false
	if _, err := os.Stat("/sys/module/morse"); err == nil {
		morseLoaded = true
	}

	if !morseLoaded || curScheme != targetScheme {
		fmt.Printf("[INFO] Morse HaLow AP 需要 scheme=%s (%s), 目前為 scheme=%s (morse: %v), 正在重新載入驅動...\n",
			targetScheme, country, curScheme, morseLoaded)

		if morseLoaded {
			_ = UnloadDriver(h.module)
		}

		h.module.Kernel.PreloadInsmodArgs = fmt.Sprintf("channelization_scheme=%s", targetScheme)
		h.module.Kernel.InsmodArgs = fmt.Sprintf("country=%s bcf=bcf_default.bin", country)

		if err := LoadDriver(h.module); err != nil {
			fmt.Printf("[WARN] 自動重新載入 Morse 驅動失敗: %v\n", err)
		} else {
			fmt.Println("[INFO] Morse 驅動已就緒並套用正確 Channelization Scheme")
		}
	}
}

// startHostapd starts the hostapd daemon
func (h *Hotspot) startHostapd(logPath string) error {
	tw, err := logger.NewTimestampWriter(logPath)
	if err != nil {
		return fmt.Errorf("failed to create hostapd log file: %w", err)
	}
	h.hostapdLog = tw

	args := []string{"-dd", h.hostapdConfig}
	h.hostapdCmd = exec.Command(h.hostapdPath, args...)
	h.hostapdCmd.Stdout = tw
	h.hostapdCmd.Stderr = tw

	// Print full command
	cmdStr := fmt.Sprintf("%s %s", h.hostapdPath, strings.Join(args, " "))
	fmt.Printf("[CMD] %s\n", cmdStr)

	if err := h.hostapdCmd.Start(); err != nil {
		tw.Close()
		h.hostapdLog = nil
		return fmt.Errorf("failed to start hostapd: %w", err)
	}

	fmt.Println(i18n.T("hotspot.hostapd_started"))
	return nil
}

// startDnsmasq starts the dnsmasq daemon
func (h *Hotspot) startDnsmasq(logPath string) error {
	tw, err := logger.NewTimestampWriter(logPath)
	if err != nil {
		return fmt.Errorf("failed to create dnsmasq log file: %w", err)
	}
	h.dnsmasqLog = tw

	// -k: keep in foreground, -C: config file, --log-facility=-: log to stderr
	args := []string{"-k", "-C", h.dnsmasqConfig, "--log-facility=-"}
	h.dnsmasqCmd = exec.Command(h.dnsmasqPath, args...)
	h.dnsmasqCmd.Stdout = tw
	h.dnsmasqCmd.Stderr = tw

	// Print full command
	cmdStr := fmt.Sprintf("%s %s", h.dnsmasqPath, strings.Join(args, " "))
	fmt.Printf("[CMD] %s\n", cmdStr)

	if err := h.dnsmasqCmd.Start(); err != nil {
		tw.Close()
		h.dnsmasqLog = nil
		return fmt.Errorf("failed to start dnsmasq: %w", err)
	}

	fmt.Println(i18n.T("hotspot.dnsmasq_started"))
	return nil
}

// showAPInfo displays AP configuration details
func (h *Hotspot) showAPInfo() {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("hotspot.info_title") + " ==========" + ui.Reset)

	content, err := os.ReadFile(h.hostapdConfig)
	if err != nil {
		fmt.Println(i18n.Tf("hotspot.config_read_err", err))
		return
	}

	configMap := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
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

	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.iface"), 16), configMap["interface"])
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.driver"), 16), configMap["driver"])
	fmt.Printf(ui.Bold+"  %s "+ui.Cyan+"%s"+ui.Reset+"\n", ui.AlignLabel("SSID:", 16), configMap["ssid"])

	channel := configMap["channel"]
	band := "2.4GHz"
	modeStr := ""
	
	if configMap["ieee80211ah"] == "1" {
		band = "Sub-GHz (802.11ah HaLow)"
		modeStr = "802.11ah HaLow"
	} else if configMap["hw_mode"] == "g" || configMap["hw_mode"] == "b" {
		band = "2.4GHz"
		if configMap["ieee80211be"] == "1" {
			modeStr = "802.11be WiFi 7"
		} else if configMap["ieee80211ax"] == "1" {
			modeStr = "802.11ax WiFi 6"
		} else if configMap["ieee80211n"] == "1" {
			modeStr = "802.11n WiFi 4"
		} else {
			modeStr = "802.11b/g"
		}
	} else if configMap["hw_mode"] == "a" {
		// hw_mode == "a" (5GHz or 6GHz)
		is6GHz := configMap["op_class"] == "131" || configMap["op_class"] == "132" || configMap["op_class"] == "133" || configMap["op_class"] == "134" || configMap["op_class"] == "135" || configMap["op_class"] == "136" || configMap["op_class"] == "137" || channel == "37"

		if is6GHz {
			band = "6GHz"
		} else {
			band = "5GHz"
		}

		if configMap["ieee80211be"] == "1" || configMap["op_class"] == "137" {
			bw := "20MHz"
			if configMap["op_class"] == "137" || configMap["eht_oper_chwidth"] == "3" {
				bw = "320MHz"
			} else if configMap["op_class"] == "134" || configMap["op_class"] == "135" || configMap["eht_oper_chwidth"] == "2" {
				bw = "160MHz"
			} else if configMap["op_class"] == "133" || configMap["eht_oper_chwidth"] == "1" || configMap["vht_oper_chwidth"] == "1" || configMap["he_oper_chwidth"] == "1" {
				bw = "80MHz"
			} else if configMap["op_class"] == "132" || strings.Contains(configMap["ht_capab"], "HT40") {
				bw = "40MHz"
			}
			modeStr = fmt.Sprintf("802.11be WiFi 7 (%s)", bw)
		} else if configMap["ieee80211ax"] == "1" {
			bw := "20MHz"
			if configMap["op_class"] == "134" || configMap["op_class"] == "135" || configMap["he_oper_chwidth"] == "2" {
				bw = "160MHz"
			} else if configMap["op_class"] == "133" || configMap["he_oper_chwidth"] == "1" || configMap["vht_oper_chwidth"] == "1" {
				bw = "80MHz"
			} else if configMap["op_class"] == "132" || strings.Contains(configMap["ht_capab"], "HT40") {
				bw = "40MHz"
			}
			modeStr = fmt.Sprintf("802.11ax WiFi 6 (%s)", bw)
		} else if configMap["ieee80211ac"] == "1" {
			modeStr = "802.11ac WiFi 5"
		} else if configMap["ieee80211n"] == "1" {
			modeStr = "802.11n WiFi 4"
		} else {
			modeStr = "802.11a"
		}
	}

	centerCh := ""
	if c := configMap["eht_oper_centr_freq_seg0_idx"]; c != "" {
		centerCh = c
	} else if c := configMap["he_oper_centr_freq_seg0_idx"]; c != "" {
		centerCh = c
	} else if c := configMap["vht_oper_centr_freq_seg0_idx"]; c != "" {
		centerCh = c
	}

	chDisplay := fmt.Sprintf("channel %s", channel)
	if centerCh != "" && centerCh != channel {
		chDisplay = fmt.Sprintf("Center: %s, Primary: %s", centerCh, channel)
	}

	if modeStr != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s [%s] (%s)"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.band_mode"), 16), band, modeStr, chDisplay)
	} else {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s (%s)"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.band"), 16), band, chDisplay)
	}

	security := i18n.T("hotspot.open_network")
	if configMap["wpa"] == "2" || configMap["wpa"] == "3" {
		keyMgmt := configMap["wpa_key_mgmt"]
		pairwise := configMap["rsn_pairwise"]
		if pairwise == "" {
			pairwise = configMap["wpa_pairwise"]
		}
		if strings.Contains(keyMgmt, "SAE") {
			security = fmt.Sprintf("WPA3-SAE (%s)", pairwise)
		} else if keyMgmt != "" && keyMgmt != "WPA-PSK" {
			security = fmt.Sprintf("%s (%s)", keyMgmt, pairwise)
		} else {
			security = fmt.Sprintf("WPA2-PSK (%s)", pairwise)
		}
	}
	fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.security"), 16), security)

	if configMap["country_code"] != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.country"), 16), configMap["country_code"])
	}

	// Read DHCP info from dnsmasq.conf
	gateway := "N/A"
	dhcpRange := "N/A"
	if dnsData, err := os.ReadFile(h.dnsmasqConfig); err == nil {
		for _, line := range strings.Split(string(dnsData), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "listen-address=") {
				gateway = strings.TrimPrefix(line, "listen-address=")
			} else if strings.HasPrefix(line, "dhcp-range=") {
				dhcpRange = strings.TrimPrefix(line, "dhcp-range=")
			}
		}
	}
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.gateway"), 16), gateway)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("DHCP:", 16), dhcpRange)

	// Show iw interface details
	ShowInterfaceDetail(h.module.Kernel.GetInterface())

	fmt.Println(ui.Blue + ui.Bold + "==================================" + ui.Reset)
}

// saveDmesg saves dmesg output to log file
func saveDmesg(logPath, phase string) {
	// Try dmesg -T (human-readable timestamps), fallback to plain dmesg
	cmd := exec.Command("dmesg", "-T")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		cmd = exec.Command("dmesg")
		output, err = cmd.Output()
		if err != nil {
			fmt.Println(i18n.Tf("hotspot.dmesg_fail", err))
			return
		}
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(i18n.Tf("hotspot.dmesg_write_fail", err))
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("\n=== dmesg [%s] at %s ===\n", phase, timestamp))
	f.Write(output)
}

// Stop stops all services
func (h *Hotspot) Stop() error {
	// Kill tracked processes first
	if h.dnsmasqCmd != nil && h.dnsmasqCmd.Process != nil {
		fmt.Printf("[CMD] kill %d (dnsmasq)\n", h.dnsmasqCmd.Process.Pid)
		h.dnsmasqCmd.Process.Kill()
	}
	if h.dnsmasqLog != nil {
		h.dnsmasqLog.Close()
		h.dnsmasqLog = nil
	}

	if h.hostapdCmd != nil && h.hostapdCmd.Process != nil {
		fmt.Printf("[CMD] kill %d (hostapd)\n", h.hostapdCmd.Process.Pid)
		h.hostapdCmd.Process.Kill()
	}
	if h.hostapdLog != nil {
		h.hostapdLog.Close()
		h.hostapdLog = nil
	}

	// Kill any remaining system-wide instances
	exec.Command("killall", "-9", "wpa_supplicant").Run()
	exec.Command("killall", "-9", "wpa_supplicant_s1g").Run()
	exec.Command("killall", "-9", "hostapd").Run()
	exec.Command("killall", "-9", "hostapd_s1g").Run()
	exec.Command("killall", "-9", "dnsmasq").Run()

	// Tear down the wireless interface to fully remove the AP
	iface := h.module.Kernel.GetInterface()
	fmt.Printf("[CMD] ip addr flush dev %s\n", iface)
	exec.Command("ip", "addr", "flush", "dev", iface).Run()
	fmt.Printf("[CMD] ip link set %s down\n", iface)
	exec.Command("ip", "link", "set", iface, "down").Run()

	fmt.Println(i18n.Tf("hotspot.all_stopped", iface))

	return nil
}
