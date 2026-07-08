package wifi

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"qwifitesttool/internal/config"
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
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== AP 模式 (熱點) ==========" + ui.Reset)

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
		// Interactive wizard
		ssid, password, band, err := h.promptConfig()
		if err != nil {
			return err
		}

		configContent := config.GenerateHostapdConfig(h.module, ssid, password, band)
		if err := config.SaveConfig(h.hostapdConfig, configContent); err != nil {
			return err
		}
		fmt.Printf("[INFO] 已生成 hostapd 配置: %s\n", h.hostapdConfig)

		if confMgr.PromptSaveCustom() {
			confMgr.SaveAsCustomConfig(configContent)
		}
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
		fmt.Printf("[INFO] 已生成 dnsmasq 配置: %s\n", h.dnsmasqConfig)

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
		fmt.Printf("[WARN] 無法讀取 %s 配置: %v\n", name, err)
		return
	}

	fmt.Printf("\n[INFO] 載入 %s 配置: %s\n", name, path)
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

// promptConfig prompts user for hotspot configuration
func (h *Hotspot) promptConfig() (ssid, password, band string, err error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("輸入熱點名稱 (SSID): ")
	ssid, _ = reader.ReadString('\n')
	ssid = strings.TrimSpace(ssid)
	if ssid == "" {
		return "", "", "", fmt.Errorf("SSID cannot be empty")
	}

	fmt.Print("輸入密碼 (至少 8 位，留空表示開放網路): ")
	password, _ = reader.ReadString('\n')
	password = strings.TrimSpace(password)

	if password != "" && len(password) < 8 {
		return "", "", "", fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("選擇頻段 [2.4G/5G] (預設 2.4G): ")
	band, _ = reader.ReadString('\n')
	band = strings.TrimSpace(band)
	if band == "" {
		band = "2.4G"
	}

	return ssid, password, band, nil
}

// startServices starts hostapd and dnsmasq with logging
func (h *Hotspot) startServices() error {
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

	// Setup interface IP before starting dnsmasq
	if err := h.setupInterfaceIP(); err != nil {
		fmt.Printf("[WARN] 設定介面 IP 失敗: %v\n", err)
	}

	// Set regulatory domain before starting hostapd
	h.setRegulatoryDomain()

	// Start hostapd
	if err := h.startHostapd(hostapdLog); err != nil {
		return err
	}

	// Wait for hostapd to initialize
	time.Sleep(2 * time.Second)

	// Start dnsmasq
	if err := h.startDnsmasq(dnsmasqLog); err != nil {
		fmt.Printf("[WARN] 啟動 dnsmasq 失敗: %v\n", err)
		// Continue anyway, hostapd can work without dnsmasq
	}

	// Display AP information
	h.showAPInfo()

	fmt.Println("\n[INFO] 日誌檔案:")
	fmt.Printf("  hostapd: %s\n", hostapdLog)
	fmt.Printf("  dnsmasq: %s\n", dnsmasqLog)
	fmt.Printf("  dmesg:   %s\n", dmesgLog)
	fmt.Println("\n按 Ctrl+C 停止熱點...")

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal or process exit
	done := make(chan error, 1)
	go func() {
		if h.hostapdCmd != nil {
			done <- h.hostapdCmd.Wait()
		}
	}()

	select {
	case <-sigChan:
		fmt.Println("\n[INFO] 正在停止熱點...")
	case err := <-done:
		if err != nil {
			fmt.Printf("[WARN] hostapd 意外終止: %v\n", err)
		}
	}

	// Stop all services
	h.Stop()

	// Save dmesg after stopping
	saveDmesg(dmesgLog, "after")

	fmt.Println("[INFO] 熱點已停止")
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

	fmt.Printf("[INFO] 設定 regulatory domain: %s\n", countryCode)
	cmd := exec.Command("iw", "reg", "set", countryCode)
	cmdStr := fmt.Sprintf("iw reg set %s", countryCode)
	fmt.Printf("[CMD] %s\n", cmdStr)
	if err := cmd.Run(); err != nil {
		fmt.Printf("[WARN] 設定 regulatory domain 失敗: %v\n", err)
	}

	// Give the kernel time to apply the new regulatory domain
	time.Sleep(500 * time.Millisecond)
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

	fmt.Println("[INFO] hostapd 已啟動")
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

	fmt.Println("[INFO] dnsmasq 已啟動 (DHCP/DNS)")
	return nil
}

// showAPInfo displays AP configuration details
func (h *Hotspot) showAPInfo() {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== AP 詳細資訊 ==========" + ui.Reset)

	content, err := os.ReadFile(h.hostapdConfig)
	if err != nil {
		fmt.Printf(ui.Yellow+"[WARN] "+ui.Reset+"無法讀取配置: %v\n", err)
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

	fmt.Printf(ui.Bold+"  %-10s "+ui.Green+"%s"+ui.Reset+"\n", "介面:", configMap["interface"])
	fmt.Printf(ui.Bold+"  %-10s "+ui.Green+"%s"+ui.Reset+"\n", "驅動:", configMap["driver"])
	fmt.Printf(ui.Bold+"  %-10s "+ui.Cyan+"%s"+ui.Reset+"\n", "SSID:", configMap["ssid"])

	channel := configMap["channel"]
	band := "2.4GHz"
	if configMap["ieee80211ah"] == "1" {
		band = "Sub-GHz (802.11ah HaLow)"
	} else if configMap["hw_mode"] == "a" {
		band = "5GHz"
	}
	fmt.Printf(ui.Bold+"  %-10s "+ui.Green+"%s (channel %s)"+ui.Reset+"\n", "頻段:", band, channel)

	security := "開放網路"
	if configMap["wpa"] == "2" {
		keyMgmt := configMap["wpa_key_mgmt"]
		pairwise := configMap["rsn_pairwise"]
		if pairwise == "" {
			pairwise = configMap["wpa_pairwise"]
		}
		security = fmt.Sprintf("WPA2-PSK (%s)", pairwise)
		if keyMgmt != "" && keyMgmt != "WPA-PSK" {
			security = fmt.Sprintf("%s (%s)", keyMgmt, pairwise)
		}
	}
	fmt.Printf(ui.Bold+"  %-10s "+ui.Yellow+"%s"+ui.Reset+"\n", "安全性:", security)

	if configMap["country_code"] != "" {
		fmt.Printf(ui.Bold+"  %-10s "+ui.Green+"%s"+ui.Reset+"\n", "國家代碼:", configMap["country_code"])
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
	fmt.Printf(ui.Bold+"  %-10s "+ui.Green+"%s"+ui.Reset+"\n", "閘道 IP:", gateway)
	fmt.Printf(ui.Bold+"  %-10s "+ui.Green+"%s"+ui.Reset+"\n", "DHCP:", dhcpRange)

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
			fmt.Printf("[WARN] dmesg 失敗: %v\n", err)
			return
		}
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("[WARN] 無法寫入 dmesg log: %v\n", err)
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
		h.dnsmasqCmd.Wait()
	}
	if h.dnsmasqLog != nil {
		h.dnsmasqLog.Close()
		h.dnsmasqLog = nil
	}

	if h.hostapdCmd != nil && h.hostapdCmd.Process != nil {
		fmt.Printf("[CMD] kill %d (hostapd)\n", h.hostapdCmd.Process.Pid)
		h.hostapdCmd.Process.Kill()
		h.hostapdCmd.Wait()
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

	fmt.Printf("[INFO] 已停止 hostapd/dnsmasq，介面 %s 已關閉\n", iface)

	return nil
}
