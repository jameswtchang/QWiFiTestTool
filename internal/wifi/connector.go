package wifi

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"qwifitesttool/internal/config"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"strings"
	"time"
)

// Connector handles Station mode operations (client mode)
type Connector struct {
	module        *models.WiFiModule
	wpaSupplicant string
	wpaCliPath    string
	ctrlInterface string
	configPath    string
	daemonStarted bool
}

// NewConnector creates a new connector instance
func NewConnector(module *models.WiFiModule, wpaSupplicantPath, wpaCliPath, ctrlInterface string) *Connector {
	return &Connector{
		module:        module,
		wpaSupplicant: wpaSupplicantPath,
		wpaCliPath:    wpaCliPath,
		ctrlInterface: ctrlInterface,
		daemonStarted: false,
	}
}

// Start initiates the Station mode workflow
func (c *Connector) Start() (bool, error) {
	fmt.Println("\n========== Station 模式 ==========")

	skipAutoConnect := false

	// STEP 1: Check if already connected BEFORE starting wpa_supplicant
	hasIP, ipAddr := c.checkIPAddressDirect()
	if hasIP {
		fmt.Printf("[INFO] WiFi 已連線，IP 地址: %s\n", ipAddr)

		connected, ssid := c.checkConnectionStatusDirect()
		if connected {
			fmt.Printf("[INFO] 當前連接的網路: %s\n", ssid)
			c.showConnectionInfoDirect()
		}

		fmt.Print("\n是否斷開並重新連接? [y/N]: ")
		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			fmt.Println("[INFO] 保持當前連線，不啟動新的 wpa_supplicant")
			return true, nil // Keep existing connection
		}

		// User chose to disconnect
		fmt.Println("[INFO] 斷開當前連線...")

		c.stopWpaSupplicant()
		time.Sleep(1 * time.Second)

		fmt.Println("\n[INFO] 驗證斷開狀態...")
		// Re-check status
		hasIPAfter, _ := c.checkIPAddressDirect()
		connectedAfter, _ := c.checkConnectionStatusDirect()

		if hasIPAfter || connectedAfter {
			fmt.Println("[WARN] 仍有殘留連線，執行強制清理...")
			c.forceCleanup()
			time.Sleep(1 * time.Second)
		}

		hasIPAfter, _ = c.checkIPAddressDirect()
		connectedAfter, _ = c.checkConnectionStatusDirect()

		if hasIPAfter {
			fmt.Println("  ⚠️  仍有 IP 地址")
		} else {
			fmt.Println("  ✓ IP 地址已清除")
		}

		if connectedAfter {
			fmt.Println("  ⚠️  仍顯示已連接")
		} else {
			fmt.Println("  ✓ WiFi 已斷開")
		}

		// Show menu
		fmt.Println("\n========== 重新連接選項 ==========")
		fmt.Println("[1] 使用 /etc/wpa_supplicant.conf 自動連接")
		fmt.Println("[2] 掃描網路並手動選擇")
		fmt.Println("[0] 取消")
		fmt.Print("選擇: ")

		var choice int
		fmt.Scanln(&choice)

		switch choice {
		case 1:
			fmt.Println("\n[INFO] 使用配置文件自動連接...")
			confMgr := config.NewConfigManager("wpa_supplicant", c.module)
			configPath, _, err := confMgr.GetConfigPath()
			if err != nil {
				return false, fmt.Errorf("獲取配置文件失敗: %w", err)
			}
			c.configPath = configPath
			return c.autoConnectFromConfig()
		case 2:
			fmt.Println("\n[INFO] 進入掃描模式...")
			skipAutoConnect = true
		case 0:
			fmt.Println("[INFO] 已取消")
			return false, nil
		default:
			fmt.Println("[WARN] 無效選擇，進入掃描模式...")
			skipAutoConnect = true
		}
	}

	// STEP 2: Load/prepare config
	confMgr := config.NewConfigManager("wpa_supplicant", c.module)

	configPath, exists, err := confMgr.GetConfigPath()
	if err != nil {
		return false, fmt.Errorf("獲取配置文件路徑失敗: %w", err)
	}
	c.configPath = configPath

	if !exists {
		configContent := config.GenerateWpaSupplicantConfig(c.module, c.ctrlInterface, "", "")
		if err := config.SaveConfig(c.configPath, configContent); err != nil {
			return false, fmt.Errorf("保存配置文件失敗: %w", err)
		}

		if confMgr.PromptSaveCustom() {
			confMgr.SaveAsCustomConfig(configContent)
		}
	}

	// STEP 3: Clean up and start wpa_supplicant
	if err := c.cleanupSocketFiles(); err != nil {
		fmt.Printf("[WARN] Socket cleanup warning: %v\n", err)
	}

	if err := c.startDaemon(); err != nil {
		fmt.Printf("\n❌ 錯誤: %v\n", err)

		// Ask if user wants to scan manually
		fmt.Print("\n是否要手動掃描網路? [y/N]: ")
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			// Start wpa_supplicant with minimal config for scanning
			fmt.Println("\n[INFO] 嘗試使用最小配置啟動掃描模式...")
			if err := c.startDaemonForScan(); err != nil {
				return false, fmt.Errorf("無法啟動掃描模式: %w", err)
			}
			time.Sleep(2 * time.Second)
			return c.interactiveScanConnect()
		}

		// User chose not to scan manually, return nil to avoid double error printing in main
		// since we already printed the error above.
		return false, nil
	}

	time.Sleep(2 * time.Second)

	// STEP 4: Check if config has network block for auto-connect
	// Only auto-connect if NOT in explicit scan mode
	if !skipAutoConnect && c.configHasNetwork() {
		fmt.Println("[INFO] 配置檔案中有 network 區塊，嘗試自動連接...")

		time.Sleep(5 * time.Second)

		connected, ssid := c.checkConnectionStatus()
		if connected {
			fmt.Printf("[SUCCESS] 已自動連接到: %s\n", ssid)

			hasIP, ipAddr := c.checkIPAddress()
			if !hasIP {
				fmt.Println("[INFO] 獲取 IP 地址...")
				if err := c.runDHCPClient(); err != nil {
					fmt.Printf("[WARN] DHCP 獲取 IP 失敗: %v\n", err)
				}
			} else {
				fmt.Printf("[INFO] 已有 IP 地址: %s\n", ipAddr)
			}

			c.showConnectionInfo()
			return true, nil
		}

		fmt.Println("[WARN] 自動連接失敗，進入掃描模式...")
	}

	// STEP 5: Interactive scan and connect
	keepRunning, err := c.interactiveScanConnect()
	if err != nil {
		return false, err
	}

	return keepRunning, nil
}

// startDaemon starts the wpa_supplicant daemon
func (c *Connector) startDaemon() error {
	iface := c.module.Kernel.GetInterface()
	driver := c.module.Kernel.GetDriverBackend()

	if c.configPath == "" {
		return fmt.Errorf("configPath is empty")
	}

	args := []string{
		"-i", iface,
		"-D", driver,
		"-c", c.configPath,
		"-B",
	}

	cmdStr := fmt.Sprintf("%s %s", c.wpaSupplicant, strings.Join(args, " "))
	fmt.Printf("[CMD] %s\n", cmdStr)

	cmd := exec.Command(c.wpaSupplicant, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to start wpa_supplicant: %w\nOutput: %s", err, string(output))
	}

	c.daemonStarted = true
	fmt.Println("[INFO] wpa_supplicant 已啟動")
	return nil
}

// startDaemonForScan starts wpa_supplicant with minimal config for scanning only
func (c *Connector) startDaemonForScan() error {
	iface := c.module.Kernel.GetInterface()
	driver := c.module.Kernel.GetDriverBackend()

	// Create minimal config for scanning
	minimalConfig := fmt.Sprintf(`ctrl_interface=/var/run/wpa_supplicant
update_config=1
`)
	minimalConfigPath := "/tmp/wpa_scan.conf"
	if err := os.WriteFile(minimalConfigPath, []byte(minimalConfig), 0644); err != nil {
		return fmt.Errorf("無法建立最小配置檔: %w", err)
	}

	c.configPath = minimalConfigPath

	args := []string{
		"-i", iface,
		"-D", driver,
		"-c", minimalConfigPath,
		"-B",
	}

	cmdStr := fmt.Sprintf("%s %s", c.wpaSupplicant, strings.Join(args, " "))
	fmt.Printf("[CMD] %s\n", cmdStr)

	cmd := exec.Command(c.wpaSupplicant, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("無法啟動掃描模式: %w\nOutput: %s", err, string(output))
	}

	c.daemonStarted = true
	fmt.Println("[INFO] wpa_supplicant 已啟動 (掃描模式)")
	return nil
}

// interactiveScanConnect performs scan and prompts user to connect
func (c *Connector) interactiveScanConnect() (bool, error) {
	// 1. Scan
	scanner := NewScanner(c.wpaCliPath, c.module.Kernel.GetInterface())
	networks, err := scanner.Scan()
	if err != nil {
		return false, fmt.Errorf("掃描失敗: %w (請確認 wpa_supplicant 已啟動)", err)
	}

	PrintScanResults(networks)

	if len(networks) == 0 {
		return false, nil
	}

	// 2. Select
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("選擇網路編號: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var selectedIndex int
	fmt.Sscanf(input, "%d", &selectedIndex)

	if selectedIndex < 1 || selectedIndex > len(networks) {
		return false, fmt.Errorf("invalid selection")
	}

	selected := networks[selectedIndex-1]

	// 3. Password
	fmt.Print("輸入密碼 (留空表示開放網路): ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	// 4. Connect
	return false, c.connectToNetwork(selected.SSID, password)
}

// connectToNetwork adds network and connects
func (c *Connector) connectToNetwork(ssid, password string) error {
	// If we are using temporary scan config, verify and switch back to official config
	if c.configPath == "/tmp/wpa_scan.conf" {
		confMgr := config.NewConfigManager("wpa_supplicant", c.module)
		if path, _, err := confMgr.GetConfigPath(); err == nil {
			c.configPath = path
			fmt.Printf("[INFO] 切換回正式配置檔: %s\n", c.configPath)
		}
	}

	iface := c.module.Kernel.GetInterface()

	fmt.Printf("[INFO] 連接到網路: %s\n", ssid)
	fmt.Println("\n[DEBUG] ========== 開始配置網路 ==========")

	fmt.Println("[INFO] 停止現有 wpa_supplicant...")
	c.Stop()
	time.Sleep(1 * time.Second)

	fmt.Printf("[INFO] 更新配置文件: %s\n", c.configPath)
	if err := updateWpaSupplicantConfig(c.configPath, ssid, password); err != nil {
		return fmt.Errorf("更新配置失敗: %w", err)
	}

	printConfigFile(c.configPath)

	fmt.Println("\n[INFO] 使用新配置重啟 wpa_supplicant...")
	if err := c.startDaemon(); err != nil {
		return err
	}

	fmt.Println("[DEBUG] ========== 配置完成 ==========\n")

	fmt.Println("[INFO] 等待連接...")
	time.Sleep(8 * time.Second)

	hasIP, ipAddr := c.checkIPAddress()
	if hasIP {
		fmt.Printf("✓ 已有 IP 地址: %s\n", ipAddr)
	} else {
		fmt.Println("[INFO] 獲取 IP 地址...")
		if err := c.runDHCPClient(); err != nil {
			fmt.Printf("[WARN] DHCP 獲取 IP 失敗: %v\n", err)
			fmt.Println("[INFO] 您可能需要手動配置 IP 或重試")
		}
	}

	time.Sleep(2 * time.Second)

	connected, connSSID := c.checkConnectionStatus()
	hasIP, ipAddr = c.checkIPAddress()

	if connected && hasIP {
		fmt.Printf("\n✓ 連接成功!\n")
		fmt.Printf("[INFO] 已連接到: %s\n", connSSID)
		fmt.Printf("[INFO] IP 地址: %s\n", ipAddr)
		c.showConnectionInfo()
	} else if !connected {
		fmt.Println("\n⚠️  連接失敗，診斷資訊：")
		statusCmd := exec.Command(c.wpaCliPath, "-i", iface, "status")
		statusOutput, _ := statusCmd.Output()
		c.diagnoseConnectionFailure(statusOutput, ssid, password)
	} else {
		fmt.Println("\n⚠️  已連接但未獲取 IP")
		fmt.Printf("[INFO] 已連接到: %s\n", connSSID)
		fmt.Println("[INFO] 請手動執行: udhcpc -i wlan0")
	}

	return nil
}

// Stop stops the wpa_supplicant daemon
func (c *Connector) Stop() error {
	if !c.daemonStarted {
		return nil
	}

	pkillPath := sys.FindCommand("pkill", "pkill")
	cmd := exec.Command(pkillPath, "wpa_supplicant")
	cmd.Run()

	c.daemonStarted = false
	fmt.Println("[INFO] wpa_supplicant 已停止")
	return nil
}

// cleanupSocketFiles removes existing socket files
func (c *Connector) cleanupSocketFiles() error {
	iface := c.module.Kernel.GetInterface()

	socketPaths := []string{
		fmt.Sprintf("/run/wpa_supplicant/%s", iface),
		fmt.Sprintf("/var/run/wpa_supplicant/%s", iface),
		fmt.Sprintf("%s/%s", c.ctrlInterface, iface),
	}

	for _, socketPath := range socketPaths {
		if _, err := os.Stat(socketPath); err == nil {
			fmt.Printf("[INFO] 清理舊 socket: %s\n", socketPath)
			if err := os.Remove(socketPath); err != nil {
				return fmt.Errorf("無法刪除 socket %s: %w", socketPath, err)
			}
		}
	}

	return nil
}

// runDHCPClient runs DHCP client
func (c *Connector) runDHCPClient() error {
	iface := c.module.Kernel.GetInterface()

	udhcpcPath := sys.FindCommand("udhcpc", "/sbin/udhcpc")
	args := []string{"-i", iface, "-n", "-q", "-t", "5"}

	fmt.Printf("[CMD] %s -i %s -n -q -t 5\n", udhcpcPath, iface)
	cmd := exec.Command(udhcpcPath, args...)
	if err := cmd.Run(); err == nil {
		fmt.Println("[SUCCESS] IP 地址已獲取")
		return nil
	}

	dhclientPath := sys.FindCommand("dhclient", "/sbin/dhclient")
	fmt.Printf("[CMD] %s %s\n", dhclientPath, iface)
	cmd = exec.Command(dhclientPath, iface)
	if err := cmd.Run(); err == nil {
		fmt.Println("[SUCCESS] IP 地址已獲取")
		return nil
	}

	return fmt.Errorf("所有 DHCP 客戶端嘗試失敗")
}

// diagnoseConnectionFailure analyzes why connection failed
func (c *Connector) diagnoseConnectionFailure(statusOutput []byte, ssid, password string) {
	iface := c.module.Kernel.GetInterface()
	statusStr := string(statusOutput)

	fmt.Println("\n========== 連接失敗診斷 ==========")

	fmt.Println("當前狀態:")
	lines := strings.Split(statusStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "wpa_state=") ||
			strings.HasPrefix(line, "ssid=") ||
			strings.HasPrefix(line, "key_mgmt=") {
			fmt.Printf("  %s\n", line)
		}
	}

	fmt.Println("\n可能原因：")
	reasons := []string{}

	if password != "" {
		reasons = append(reasons, "1. 密碼錯誤 - 請確認密碼是否正確")
	}

	cmd := exec.Command(c.wpaCliPath, "-i", iface, "signal_poll")
	if output, err := cmd.Output(); err == nil {
		if strings.Contains(string(output), "RSSI=") {
			reasons = append(reasons, "2. 信號太弱 - RSSI 值過低")
		}
	}

	if strings.Contains(statusStr, "wpa_state=DISCONNECTED") {
		if password != "" {
			reasons = append(reasons, "3. 認證超時 - 可能密碼錯誤或 AP 拒絕連接")
		} else {
			reasons = append(reasons, "3. AP 可能需要密碼，但您輸入的是開放網路")
		}
	}

	if len(reasons) == 0 {
		fmt.Println("  未能確定具體原因，請檢查 SSID 和範圍")
	} else {
		for _, reason := range reasons {
			fmt.Printf("  %s\n", reason)
		}
	}
	fmt.Println("==============================")
}
