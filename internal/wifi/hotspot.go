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
	"qwifitesttool/internal/models"
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
}

// NewHotspot creates a new hotspot instance
func NewHotspot(module *models.WiFiModule, hostapdPath string) *Hotspot {
	return &Hotspot{
		module:      module,
		hostapdPath: hostapdPath,
		dnsmasqPath: "dnsmasq", // Will be resolved dynamically
		logDir:      "/tmp/ap_logs",
	}
}

// Start initiates the AP mode workflow
func (h *Hotspot) Start() error {
	fmt.Println("\n========== AP 模式 (熱點) ==========")

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
	dmesgLog := filepath.Join(h.logDir, fmt.Sprintf("dmesg_%s.log", timestamp))

	// Save dmesg before starting
	h.saveDmesg(dmesgLog, "before")

	// Setup interface IP before starting dnsmasq
	if err := h.setupInterfaceIP(); err != nil {
		fmt.Printf("[WARN] 設定介面 IP 失敗: %v\n", err)
	}

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
	h.saveDmesg(dmesgLog, "after")

	fmt.Println("[INFO] 熱點已停止")
	return nil
}

// setupInterfaceIP sets the IP address on the wireless interface
func (h *Hotspot) setupInterfaceIP() error {
	iface := h.module.Kernel.GetInterface()
	gatewayIP := "192.168.50.1/24"

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

// startHostapd starts the hostapd daemon
func (h *Hotspot) startHostapd(logPath string) error {
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create hostapd log file: %w", err)
	}

	args := []string{"-dd", h.hostapdConfig}
	h.hostapdCmd = exec.Command(h.hostapdPath, args...)
	h.hostapdCmd.Stdout = logFile
	h.hostapdCmd.Stderr = logFile

	// Print full command
	cmdStr := fmt.Sprintf("%s %s", h.hostapdPath, strings.Join(args, " "))
	fmt.Printf("[CMD] %s\n", cmdStr)

	if err := h.hostapdCmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start hostapd: %w", err)
	}

	fmt.Println("[INFO] hostapd 已啟動")
	return nil
}

// startDnsmasq starts the dnsmasq daemon
func (h *Hotspot) startDnsmasq(logPath string) error {
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create dnsmasq log file: %w", err)
	}

	// -k: keep in foreground, -C: config file, --log-facility: log to file
	args := []string{"-k", "-C", h.dnsmasqConfig, "--log-facility=" + logPath}
	h.dnsmasqCmd = exec.Command(h.dnsmasqPath, args...)
	h.dnsmasqCmd.Stdout = logFile
	h.dnsmasqCmd.Stderr = logFile

	// Print full command
	cmdStr := fmt.Sprintf("%s %s", h.dnsmasqPath, strings.Join(args, " "))
	fmt.Printf("[CMD] %s\n", cmdStr)

	if err := h.dnsmasqCmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start dnsmasq: %w", err)
	}

	fmt.Println("[INFO] dnsmasq 已啟動 (DHCP/DNS)")
	return nil
}

// showAPInfo displays AP configuration details
func (h *Hotspot) showAPInfo() {
	fmt.Println("\n========== AP 詳細資訊 ==========")

	content, err := os.ReadFile(h.hostapdConfig)
	if err != nil {
		fmt.Printf("[WARN] 無法讀取配置: %v\n", err)
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

	fmt.Printf("  介面:     %s\n", configMap["interface"])
	fmt.Printf("  驅動:     %s\n", configMap["driver"])
	fmt.Printf("  SSID:     %s\n", configMap["ssid"])

	hwMode := configMap["hw_mode"]
	channel := configMap["channel"]
	band := "2.4GHz"
	if hwMode == "a" {
		band = "5GHz"
	}
	fmt.Printf("  頻段:     %s (channel %s)\n", band, channel)

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
	fmt.Printf("  安全性:   %s\n", security)

	// DHCP info
	fmt.Printf("  閘道 IP:  192.168.50.1\n")
	fmt.Printf("  DHCP:     192.168.50.10 - 192.168.50.100\n")

	if configMap["country_code"] != "" {
		fmt.Printf("  國家代碼: %s\n", configMap["country_code"])
	}

	// Show iw interface details
	ShowInterfaceDetail(h.module.Kernel.GetInterface())

	fmt.Println("==================================")
}

// saveDmesg saves dmesg output to log file
func (h *Hotspot) saveDmesg(logPath, phase string) {
	cmd := exec.Command("dmesg", "-T")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("\n=== dmesg [%s] at %s ===\n", phase, timestamp))
	f.Write(output)
}

// Stop stops all services
func (h *Hotspot) Stop() error {
	if h.dnsmasqCmd != nil && h.dnsmasqCmd.Process != nil {
		fmt.Printf("[CMD] kill -9 %d (dnsmasq)\n", h.dnsmasqCmd.Process.Pid)
		h.dnsmasqCmd.Process.Kill()
		fmt.Println("[INFO] dnsmasq 已停止")
	}
	if h.hostapdCmd != nil && h.hostapdCmd.Process != nil {
		fmt.Printf("[CMD] kill -9 %d (hostapd)\n", h.hostapdCmd.Process.Pid)
		h.hostapdCmd.Process.Kill()
		fmt.Println("[INFO] hostapd 已停止")
	}
	return nil
}
