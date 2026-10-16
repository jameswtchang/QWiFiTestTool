package wifi

import (
	"errors"
	"fmt"
	"net"
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
	"qwifitesttool/internal/sys"
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
	svc           *sys.ServiceManager // Android framework takeover (nil-safe no-op on Linux)
}

// NewHotspot creates a new hotspot instance
func NewHotspot(module *models.WiFiModule, hostapdPath string, svc *sys.ServiceManager) *Hotspot {
	return &Hotspot{
		module:      module,
		hostapdPath: hostapdPath,
		dnsmasqPath: sys.FindCommand("dnsmasq", "/usr/sbin/dnsmasq"),
		logDir:      "/tmp/wifi_logs",
		svc:         svc,
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
		fmt.Println(i18n.T("hotspot.no_config_guide"))
		
		editor, err := config.NewAPConfigEditor(h.module)
		if err != nil {
			return fmt.Errorf(i18n.T("hotspot.editor_init_fail"), err)
		}
		if err := editor.Start(); err != nil {
			return fmt.Errorf(i18n.T("hotspot.editor_fail"), err)
		}
		
		// 編輯器完成後，重新取得實際生成的配置路徑
		configPath, exists, err = confMgr.GetConfigPath()
		if err != nil || !exists {
			return errors.New(i18n.T("hotspot.no_valid_config"))
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
	// Take over WiFi from the Android framework; otherwise it keeps the interface in managed mode
	if err := h.svc.DisableAndroidWiFi(); err != nil {
		fmt.Printf("[WARN] Android WiFi takeover failed: %v\n", err)
	}

	// Clean up conflicting daemons on this interface before starting AP
	// (instances explicitly bound only to other interfaces are kept)
	h.killInterfaceDaemons(h.module.Kernel.GetInterface())
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
		if err := h.ensureMorseChannelizationScheme(); err != nil {
			return err
		}
	}

	// Setup interface IP before starting dnsmasq
	if err := h.setupInterfaceIP(); err != nil {
		fmt.Println(i18n.Tf("hotspot.ip_fail", err))
	}

	// Set regulatory domain before starting hostapd
	h.setRegulatoryDomain()

	// Handle Ctrl+C gracefully from the moment daemons start
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Start hostapd
	if err := h.startHostapd(hostapdLog); err != nil {
		return err
	}

	// Wait for hostapd to initialize.
	// PCIe/CNSS based modules (Qualcomm) need extra time for txpower initialization
	// after beacon is set; USB/SDIO modules (Realtek, MTK) may report 0.00 dBm permanently.
	// DFS channels require Channel Availability Check (CAC) which typically takes 60 seconds.
	isPCIe := strings.Contains(h.module.Kernel.PreloadKo, "pcie") || strings.Contains(h.module.Kernel.PreloadKo, "cnss")
	apReady := false
	maxWait := 15
	isDFS := false
	if data, err := os.ReadFile(h.hostapdConfig); err == nil && strings.Contains(string(data), "ieee80211h=1") {
		isDFS = true
		maxWait = 75 // CAC 60s + buffer for DFS channels
		fmt.Println(ui.Yellow + i18n.T("hotspot.dfs_cac_wait") + ui.Reset)
	}
	for i := 0; i < maxWait; i++ {
		select {
		case <-sigChan:
			fmt.Println("\n" + i18n.T("hotspot.stopping"))
			h.Stop()
			return nil
		case <-time.After(1 * time.Second):
		}

		// Without iw (e.g. stock Android) the nl80211 iftype cannot be queried;
		// fall back to hostapd's own state machine as the readiness signal.
		if !iwAvailable() {
			if h.hostapdEnabled(hostapdLog) {
				apReady = true
				break
			}
			continue
		}

		out, err := exec.Command(iwPath(), "dev", h.module.Kernel.GetInterface(), "info").Output()
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
		// During CAC the netdev is already "type AP" but not beaconing yet.
		// Gate on hostapd's own state machine instead of nl80211 iftype.
		if isDFS && h.dfsCACPending(hostapdLog, i) {
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

// netmaskToPrefix converts a dotted IPv4 netmask (e.g. 255.255.255.128) to its prefix
// length (25), rejecting non-contiguous or malformed masks so the interface address
// never disagrees with the subnet dnsmasq hands out.
func netmaskToPrefix(mask string) (int, error) {
	ip := net.ParseIP(strings.TrimSpace(mask)).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid netmask %q in XML <Network>", mask)
	}
	ones, bits := net.IPMask(ip).Size()
	if bits == 0 {
		return 0, fmt.Errorf("non-contiguous netmask %q in XML <Network>", mask)
	}
	return ones, nil
}

// setupInterfaceIP sets the IP address on the wireless interface
func (h *Hotspot) setupInterfaceIP() error {
	iface := h.module.Kernel.GetInterface()

	// Read gateway from XML Network config, fallback to default
	gateway := "192.168.50.1"
	netmask := 24
	if nw := h.module.APConfig.Network; nw != nil && nw.Gateway != "" {
		gateway = nw.Gateway
		if nw.Netmask != "" {
			prefix, err := netmaskToPrefix(nw.Netmask)
			if err != nil {
				return err
			}
			netmask = prefix
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
	cmd := exec.Command(iwPath(), "reg", "set", countryCode)
	cmdStr := fmt.Sprintf("iw reg set %s", countryCode)
	fmt.Printf("[CMD] %s\n", cmdStr)
	if err := cmd.Run(); err != nil {
		fmt.Println(i18n.Tf("hotspot.regdomain_fail", err))
	}

	// Give the kernel time to apply the new regulatory domain
	time.Sleep(500 * time.Millisecond)
}

// ensureMorseChannelizationScheme verifies and updates dot11ah channelization_scheme for Morse HaLow AP
func (h *Hotspot) ensureMorseChannelizationScheme() error {
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

	schemePath := "/sys/module/dot11ah/parameters/channelization_scheme"
	readScheme := func() string {
		if curSchemeBytes, err := os.ReadFile(schemePath); err == nil {
			return strings.TrimSpace(string(curSchemeBytes))
		}
		return ""
	}
	curScheme := readScheme()

	morseLoaded := false
	if _, err := os.Stat("/sys/module/morse"); err == nil {
		morseLoaded = true
	}
	dot11Loaded := false
	if _, err := os.Stat("/sys/module/dot11ah"); err == nil {
		dot11Loaded = true
	}

	if morseLoaded && dot11Loaded && curScheme == targetScheme {
		return nil
	}

	fmt.Println(i18n.Tf("hotspot.morse_reload", targetScheme, country, curScheme, morseLoaded, dot11Loaded))

	rmmodPath := sys.FindCommand("rmmod", "rmmod")

	if morseLoaded {
		// 必須先將網卡 down 才能成功 rmmod
		iface := h.module.Kernel.GetInterface()
		exec.Command("ip", "link", "set", iface, "down").Run()
		time.Sleep(200 * time.Millisecond)

		if err := UnloadDriver(h.module); err != nil {
			return fmt.Errorf(i18n.T("hotspot.morse_unload_fail"), err)
		}
	}

	// 確保 dot11ah 模組也被卸載
	if _, err := os.Stat("/sys/module/dot11ah"); err == nil {
		exec.Command(rmmodPath, "dot11ah").Run()
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := os.Stat("/sys/module/dot11ah"); err == nil {
		return errors.New(i18n.T("hotspot.morse_dot11ah_busy"))
	}

	// 保留除 country 之外的原有 insmod 參數（如 bcf=...）
	var otherArgs []string
	hasBcf := false
	for _, arg := range strings.Fields(h.module.Kernel.InsmodArgs) {
		if strings.HasPrefix(arg, "country=") {
			continue
		}
		if strings.HasPrefix(arg, "bcf=") {
			hasBcf = true
		}
		otherArgs = append(otherArgs, arg)
	}
	if !hasBcf {
		otherArgs = append(otherArgs, "bcf=bcf_default.bin")
	}

	h.module.Kernel.PreloadInsmodArgs = fmt.Sprintf("channelization_scheme=%s", targetScheme)
	h.module.Kernel.InsmodArgs = strings.TrimSpace(fmt.Sprintf("country=%s %s", country, strings.Join(otherArgs, " ")))

	if err := LoadDriver(h.module); err != nil {
		return fmt.Errorf(i18n.T("hotspot.morse_reload_fail"), err)
	}

	// 驗證 channelization_scheme（若 sysfs 參數可讀）
	if afterScheme := readScheme(); afterScheme != "" && afterScheme != targetScheme {
		return errors.New(i18n.Tf("hotspot.morse_scheme_mismatch", targetScheme, afterScheme))
	}

	// 等待網路介面出現（最多 8 秒）
	iface := h.module.Kernel.GetInterface()
	ifaceReady := false
	for i := 0; i < 16; i++ {
		if _, err := os.Stat("/sys/class/net/" + iface); err == nil {
			ifaceReady = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !ifaceReady {
		return errors.New(i18n.Tf("hotspot.morse_iface_missing", iface))
	}

	fmt.Println(i18n.T("hotspot.morse_ready"))
	return nil
}

// dfsCACPending reports whether hostapd is still in DFS Channel Availability Check.
// Source of truth, in order:
//  1. hostapd_cli status -> state=ENABLED / DFS (sibling of hostapdPath first, e.g. hostapd_s1g -> hostapd_cli_s1g)
//  2. hostapd log events (DFS-CAC-START without AP-ENABLED / DFS-CAC-COMPLETED)
//
// hostapd stdout is a pipe (fully buffered stdio), so completion events may lag;
// the log fallback is only trusted for the first 70s to avoid a false timeout.
func (h *Hotspot) dfsCACPending(logPath string, elapsedSec int) bool {
	if state := h.hostapdState(); state != "" {
		return state != "ENABLED"
	}

	if elapsedSec >= 70 {
		return false
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return false
	}
	logStr := string(data)
	if !strings.Contains(logStr, "DFS-CAC-START") {
		return false
	}
	return !strings.Contains(logStr, "AP-ENABLED") && !strings.Contains(logStr, "DFS-CAC-COMPLETED success=1")
}

// hostapdEnabled reports whether hostapd has reached state=ENABLED (beaconing).
// Uses hostapd_cli first, then the AP-ENABLED event in the hostapd log.
func (h *Hotspot) hostapdEnabled(logPath string) bool {
	if state := h.hostapdState(); state != "" {
		return state == "ENABLED"
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "AP-ENABLED")
}

// hostapdState queries "hostapd_cli status" and returns the state= value
// (e.g. ENABLED, DFS, COUNTRY_UPDATE), or "" if it cannot be determined.
// The hostapd_cli sibling of hostapdPath is preferred (hostapd_s1g -> hostapd_cli_s1g).
func (h *Hotspot) hostapdState() string {
	iface := h.module.Kernel.GetInterface()

	ctrlDir := ""
	if data, err := os.ReadFile(h.hostapdConfig); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ctrl_interface=") {
				ctrlDir = strings.TrimPrefix(line, "ctrl_interface=")
				ctrlDir = strings.TrimPrefix(ctrlDir, "DIR=")
				if idx := strings.Index(ctrlDir, " "); idx > 0 {
					ctrlDir = ctrlDir[:idx]
				}
				break
			}
		}
	}

	cliPath := ""
	if h.hostapdPath != "" {
		base := filepath.Base(h.hostapdPath)
		sibling := filepath.Join(filepath.Dir(h.hostapdPath), strings.Replace(base, "hostapd", "hostapd_cli", 1))
		if st, err := os.Stat(sibling); err == nil && !st.IsDir() {
			cliPath = sibling
		}
	}
	if cliPath == "" {
		cliPath = sys.FindCommandOrEmpty("hostapd_cli")
	}

	if cliPath == "" || ctrlDir == "" {
		return ""
	}
	out, err := exec.Command(cliPath, "-p", ctrlDir, "-i", iface, "status").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "state=") {
			return strings.TrimPrefix(line, "state=")
		}
	}
	return ""
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

	// ==========================================
	// Presentation / Printing
	// ==========================================

	// Group 1: Basic & System
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.iface"), 16), configMap["interface"])
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.driver"), 16), configMap["driver"])
	
	if configMap["country_code"] != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.country"), 16), configMap["country_code"])
	}

	fmt.Printf(ui.Bold+"  %s "+ui.Cyan+"%s"+ui.Reset+"\n", ui.AlignLabel("SSID:", 16), configMap["ssid"])
	fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.security"), 16), security)

	// Group 2: RF & PHY
	fmt.Println()
	if modeStr != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s [%s] (%s)"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.band_mode"), 16), band, modeStr, chDisplay)
	} else {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s (%s)"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.band"), 16), band, chDisplay)
	}
	
	// Show iw interface details (Channel, TxPower)
	ShowInterfaceDetail(h.module.Kernel.GetInterface())

	// Group 3: Network (IP/DHCP)
	fmt.Println()
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("hotspot.label.gateway"), 16), gateway)
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("DHCP:", 16), dhcpRange)

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

// killInterfaceDaemons kills wpa_supplicant/hostapd instances that may control iface
// (bound to it, or with unknown binding) and dnsmasq instances serving iface.
func (h *Hotspot) killInterfaceDaemons(iface string) {
	hostapdNames := []string{"hostapd", "hostapd_s1g"}
	if base := filepath.Base(h.hostapdPath); base != "." && base != "hostapd" && base != "hostapd_s1g" {
		hostapdNames = append(hostapdNames, base)
	}
	sys.KillDaemonsForIface([]string{"wpa_supplicant", "wpa_supplicant_s1g"}, iface)
	sys.KillDaemonsForIface(hostapdNames, iface)
	sys.KillProcsForIface([]string{"dnsmasq"}, iface)
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

	// Tear down the wireless interface to fully remove the AP
	iface := h.module.Kernel.GetInterface()

	// Kill any remaining instances on this interface
	h.killInterfaceDaemons(iface)
	fmt.Printf("[CMD] ip addr flush dev %s\n", iface)
	exec.Command("ip", "addr", "flush", "dev", iface).Run()
	fmt.Printf("[CMD] ip link set %s down\n", iface)
	exec.Command("ip", "link", "set", iface, "down").Run()

	fmt.Println(i18n.Tf("hotspot.all_stopped", iface))

	return nil
}
