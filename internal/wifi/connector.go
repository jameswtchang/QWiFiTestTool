package wifi

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"qwifitesttool/internal/config"
	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/logger"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
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
	log           *logger.SessionLogger
	wpaLog        *logger.TimestampWriter
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
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("conn.title") + " ==========" + ui.Reset)

	// Initialize session logger
	sessionLog, err := logger.NewSession("station")
	if err != nil {
		fmt.Println(i18n.Tf("conn.log_fail", err))
	} else {
		c.log = sessionLog

		// Save dmesg before starting
		dmesgPath := c.log.GetDaemonLogPath("dmesg_station")
		saveDmesg(dmesgPath, "before")

		defer func() {
			if c.log != nil {
				// Save dmesg after finishing
				saveDmesg(dmesgPath, "after")
				c.log.Close()
				fmt.Println(i18n.Tf("conn.log_saved", c.log.GetLogPath()))
				fmt.Println(i18n.Tf("conn.log_dmesg", dmesgPath))
			}
		}()
	}

	skipAutoConnect := false

	// STEP 1: Check if already connected BEFORE starting wpa_supplicant
	connected, ssid := c.checkConnectionStatusDirect()
	hasIP, ipAddr := c.checkIPAddressDirect()

	if connected {
		if hasIP {
			fmt.Println(i18n.Tf("conn.already_ip", ipAddr))
		}
		fmt.Println(i18n.Tf("conn.current_net", ssid))
		c.showConnectionInfoDirect()

		fmt.Print(i18n.T("conn.ask_reconnect"))
		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			fmt.Println(i18n.T("conn.keep_current"))
			return true, nil // Keep existing connection
		}

		// User chose to disconnect
		fmt.Println(i18n.T("conn.disconnecting2"))

		c.Stop()
		time.Sleep(1 * time.Second)

		fmt.Println("\n" + i18n.T("conn.verify_disconnect"))
		// Re-check status
		hasIPAfter, _ := c.checkIPAddressDirect()
		connectedAfter, _ := c.checkConnectionStatusDirect()

		if hasIPAfter || connectedAfter {
			fmt.Println(i18n.T("conn.residual_cleanup"))
			c.forceCleanup()
			time.Sleep(1 * time.Second)
		}

		hasIPAfter, _ = c.checkIPAddressDirect()
		connectedAfter, _ = c.checkConnectionStatusDirect()

		if hasIPAfter {
			fmt.Println(i18n.T("conn.still_ip"))
		} else {
			fmt.Println(i18n.T("conn.ip_cleared"))
		}

		if connectedAfter {
			fmt.Println(i18n.T("conn.still_connected"))
		} else {
			fmt.Println(i18n.T("conn.wifi_disconnected"))
		}

		// Show menu
		fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("conn.reconnect_title") + " ==========" + ui.Reset)
		fmt.Println(ui.Cyan + ui.Bold + "[1]" + ui.Reset + " " + i18n.T("conn.reconnect.1"))
		fmt.Println(ui.Cyan + ui.Bold + "[2]" + ui.Reset + " " + i18n.T("conn.reconnect.2"))
		fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " " + i18n.T("conn.reconnect.0"))
		fmt.Print(ui.Yellow + ui.Bold + i18n.T("conn.reconnect.select") + ui.Reset)

		var choice int
		fmt.Scanln(&choice)

		switch choice {
		case 1:
			fmt.Println("\n" + i18n.T("conn.auto_connecting"))
			confMgr := config.NewConfigManager("wpa_supplicant", c.module)
			configPath, _, err := confMgr.GetConfigPath()
			if err != nil {
				return false, fmt.Errorf(i18n.T("conn.config_get_fail"), err)
			}
			c.configPath = configPath
			return c.autoConnectFromConfig()
		case 2:
			fmt.Println("\n" + i18n.T("conn.scan_mode"))
			skipAutoConnect = true
		case 0:
			fmt.Println(i18n.T("conn.cancelled"))
			return false, nil
		default:
			fmt.Println(i18n.T("conn.invalid_scan"))
			skipAutoConnect = true
		}
	} else if hasIP {
		// Stale IP detected on interface without actual WiFi connection
		fmt.Println(i18n.Tf("conn.stale_ip_cleanup", ipAddr))
		c.forceCleanup()
	}

	// STEP 2: Load/prepare config
	confMgr := config.NewConfigManager("wpa_supplicant", c.module)

	configPath, exists, err := confMgr.GetConfigPath()
	if err != nil {
		return false, fmt.Errorf(i18n.T("conn.config_path_fail"), err)
	}
	c.configPath = configPath

	if !exists {
		configContent := config.GenerateWpaSupplicantConfig(c.module, c.ctrlInterface, "", "")
		if err := config.SaveConfig(c.configPath, configContent); err != nil {
			return false, fmt.Errorf(i18n.T("conn.config_save_fail"), err)
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
		fmt.Println("\n" + i18n.Tf("conn.daemon_fail", err))

		// Ask if user wants to scan manually
		fmt.Print(i18n.T("conn.ask_manual_scan"))
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			// Start wpa_supplicant with minimal config for scanning
			fmt.Println("\n" + i18n.T("conn.try_scan_mode"))
			if err := c.startDaemonForScan(); err != nil {
				return false, fmt.Errorf(i18n.T("conn.scan_mode_fail"), err)
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
		fmt.Println(i18n.T("conn.auto_try"))

		time.Sleep(5 * time.Second)

		connected, ssid := c.checkConnectionStatus()
		if connected {
			fmt.Println(i18n.Tf("conn.auto_ok", ssid))

			hasIP, ipAddr := c.checkIPAddress()
			if !hasIP {
				fmt.Println(i18n.T("conn.getting_ip"))
				if err := c.runDHCPClient(); err != nil {
					fmt.Println(i18n.Tf("conn.dhcp_fail", err))
				}
			} else {
				fmt.Println(i18n.Tf("conn.has_ip", ipAddr))
			}

			c.showConnectionInfo()
			return true, nil
		}

		fmt.Println(i18n.T("conn.auto_fail"))
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

	// Log config file content before starting
	if c.log != nil {
		if data, err := os.ReadFile(c.configPath); err == nil {
			c.log.LogConfig("wpa_supplicant", c.configPath, string(data))
		}
	}

	exec.Command("ip", "link", "set", iface, "down").Run()
	exec.Command("iw", "dev", iface, "set", "type", "managed").Run()

	args := []string{
		"-i", iface,
		"-D", driver,
		"-c", c.configPath,
	}

	// Add debug flag for s1g supplicant
	if strings.Contains(c.wpaSupplicant, "_s1g") {
		args = append(args, "-dd")
	}

	// Add daemon log file output via TimestampWriter
	var tw *logger.TimestampWriter
	daemonLogPath := ""
	if c.log != nil {
		daemonName := "wpa_supplicant"
		if strings.Contains(c.wpaSupplicant, "_s1g") {
			daemonName = "wpa_supplicant_s1g"
		}
		daemonLogPath = c.log.GetDaemonLogPath(daemonName)
		tw, _ = logger.NewTimestampWriter(daemonLogPath)
	}

	cmdStr := fmt.Sprintf("%s %s", c.wpaSupplicant, strings.Join(args, " "))
	fmt.Printf("[CMD] %s (Foreground)\n", cmdStr)

	if c.log != nil {
		c.log.LogDaemonStart(c.wpaSupplicant, args, daemonLogPath)
	}

	cmd := exec.Command(c.wpaSupplicant, args...)
	if tw != nil {
		cmd.Stdout = tw
		cmd.Stderr = tw
		c.wpaLog = tw
	}

	err := cmd.Start()
	
	// Keep the writer open since it's running in background as child process
	// We will rely on disconnect/cleanup to kill wpa_supplicant.
	if err != nil {
		if tw != nil {
			tw.Close()
		}
		return fmt.Errorf("failed to start wpa_supplicant: %w", err)
	}

	// Give daemon a brief moment to initialize control interface
	time.Sleep(1 * time.Second)

	if c.log != nil {
		c.log.LogCmd(cmdStr, []byte("Started in background child process"), err)
	}

	c.daemonStarted = true
	fmt.Println(i18n.T("conn.wpa_started"))
	if daemonLogPath != "" {
		fmt.Println(i18n.Tf("conn.wpa_log", daemonLogPath))
	}
	return nil
}

// startDaemonForScan starts wpa_supplicant with minimal config for scanning only
func (c *Connector) startDaemonForScan() error {
	iface := c.module.Kernel.GetInterface()
	driver := c.module.Kernel.GetDriverBackend()

	exec.Command("ip", "link", "set", iface, "down").Run()
	exec.Command("iw", "dev", iface, "set", "type", "managed").Run()

	// Create minimal config for scanning
	minimalConfig := fmt.Sprintf(`ctrl_interface=%s
update_config=1
`, c.ctrlInterface)
	minimalConfigPath := "/tmp/wpa_scan.conf"
	if err := os.WriteFile(minimalConfigPath, []byte(minimalConfig), 0644); err != nil {
		return fmt.Errorf(i18n.T("conn.min_config_fail"), err)
	}

	if c.log != nil {
		c.log.LogConfig("scan_config", minimalConfigPath, minimalConfig)
	}

	c.configPath = minimalConfigPath

	args := []string{
		"-i", iface,
		"-D", driver,
		"-c", minimalConfigPath,
	}

	// Add daemon log file output via TimestampWriter
	var tw *logger.TimestampWriter
	daemonLogPath := ""
	if c.log != nil {
		daemonLogPath = c.log.GetDaemonLogPath("wpa_supplicant_scan")
		tw, _ = logger.NewTimestampWriter(daemonLogPath)
	}

	cmdStr := fmt.Sprintf("%s %s", c.wpaSupplicant, strings.Join(args, " "))
	fmt.Printf("[CMD] %s (Foreground)\n", cmdStr)

	if c.log != nil {
		c.log.LogDaemonStart(c.wpaSupplicant, args, daemonLogPath)
	}

	cmd := exec.Command(c.wpaSupplicant, args...)
	if tw != nil {
		cmd.Stdout = tw
		cmd.Stderr = tw
		c.wpaLog = tw
	}

	err := cmd.Start()

	if err != nil {
		if tw != nil {
			tw.Close()
		}
		return fmt.Errorf(i18n.T("conn.scan_mode_fail"), err)
	}

	// Wait briefly for init
	time.Sleep(1 * time.Second)

	if c.log != nil {
		c.log.LogCmd(cmdStr, []byte("Started in background child process"), err)
	}

	c.daemonStarted = true
	fmt.Println(i18n.T("conn.wpa_scan_started"))
	return nil
}

// interactiveScanConnect performs scan and prompts user to connect
func (c *Connector) interactiveScanConnect() (bool, error) {
	// 1. Scan
	scanner := NewScanner(c.wpaCliPath, c.ctrlInterface, c.module.Kernel.GetInterface())
	networks, err := scanner.Scan()
	if err != nil {
		return false, fmt.Errorf(i18n.T("conn.scan_fail"), err)
	}

	cfgPath := c.configPath
	if cfgPath == "/tmp/wpa_scan.conf" || cfgPath == "" {
		confMgr := config.NewConfigManager("wpa_supplicant", c.module)
		if path, _, err := confMgr.GetConfigPathSilent(); err == nil {
			cfgPath = path
		}
	}

	if cfgPath != "" {
		if data, err := os.ReadFile(cfgPath); err == nil {
			policies := extractPolicyParams(string(data))
			if fl, ok := policies["freq_list"]; ok && strings.TrimSpace(fl) != "" {
				allowedFreqs := make(map[string]bool)
				for _, f := range strings.Fields(fl) {
					allowedFreqs[f] = true
				}
				if len(allowedFreqs) > 0 {
					var filtered []ScanResult
					for _, net := range networks {
						if allowedFreqs[net.Frequency] {
							filtered = append(filtered, net)
						}
					}
					fmt.Println(ui.Yellow + i18n.Tf("conn.scan_band_filter", len(filtered), len(networks)) + ui.Reset)
					networks = filtered
				}
			}
		}
	}

	PrintScanResults(networks)

	if len(networks) == 0 {
		return false, nil
	}

	// 2. Select
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(i18n.T("conn.select_network"))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var selectedIndex int
	fmt.Sscanf(input, "%d", &selectedIndex)

	if selectedIndex < 1 || selectedIndex > len(networks) {
		return false, fmt.Errorf("invalid selection")
	}

	selected := networks[selectedIndex-1]

	// 3. Password
	fmt.Print(i18n.T("conn.enter_password"))
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	// 4. Connect
	return false, c.connectToNetwork(selected.SSID, password, selected.Flags)
}

// connectToNetwork adds network and connects
func (c *Connector) connectToNetwork(ssid, password, flags string) error {
	// If we are using temporary scan config, verify and switch back to official config
	if c.configPath == "/tmp/wpa_scan.conf" {
		confMgr := config.NewConfigManager("wpa_supplicant", c.module)
		if path, _, err := confMgr.GetConfigPath(); err == nil {
			c.configPath = path
			fmt.Println(i18n.Tf("conn.switch_config", c.configPath))
		}
	}

	iface := c.module.Kernel.GetInterface()

	if c.log != nil {
		c.log.LogEvent("Connect to network: SSID=%s, flags=%s, iface=%s", ssid, flags, iface)
	}

	fmt.Println(i18n.Tf("conn.connecting_to", ssid))
	fmt.Println("\n" + i18n.T("conn.net_config_start"))

	fmt.Println(i18n.T("conn.stopping_wpa"))
	c.Stop()
	time.Sleep(1 * time.Second)

	fmt.Println(i18n.Tf("conn.update_config", c.configPath))
	if err := updateWpaSupplicantConfig(c.configPath, ssid, password, flags); err != nil {
		if c.log != nil {
			c.log.LogEvent("Config update failed: %v", err)
		}
		return fmt.Errorf(i18n.T("conn.update_config_fail"), err)
	}

	// Log updated config
	if c.log != nil {
		if data, err := os.ReadFile(c.configPath); err == nil {
			c.log.LogConfig("updated_wpa_supplicant", c.configPath, string(data))
		}
	}

	printConfigFile(c.configPath)

	fmt.Println("\n" + i18n.T("conn.restart_wpa"))
	if err := c.startDaemon(); err != nil {
		return err
	}

	fmt.Println(i18n.T("conn.net_config_done") + "\n")

	fmt.Println(i18n.T("conn.wait_connect"))
	time.Sleep(8 * time.Second)

	hasIP, ipAddr := c.checkIPAddress()
	if hasIP {
		fmt.Println(i18n.Tf("conn.already_has_ip", ipAddr))
	} else {
		fmt.Println(i18n.T("conn.getting_ip"))
		if err := c.runDHCPClient(); err != nil {
			fmt.Println(i18n.Tf("conn.dhcp_fail", err))
			fmt.Println(i18n.T("conn.need_manual_ip"))
		}
	}

	time.Sleep(2 * time.Second)

	connected, connSSID := c.checkConnectionStatus()
	hasIP, ipAddr = c.checkIPAddress()

	if connected && hasIP {
		fmt.Printf("\n%s\n", i18n.T("conn.connect_ok"))
		fmt.Println(i18n.Tf("conn.connected_to", connSSID))
		fmt.Println(i18n.Tf("conn.ip_addr", ipAddr))
		c.showConnectionInfo()
	} else if !connected {
		fmt.Println("\n" + i18n.T("conn.connect_fail_diag"))
		statusCmdStr := fmt.Sprintf("%s -p %s -i %s status", c.wpaCliPath, c.ctrlInterface, iface)
		statusCmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "status")
		statusOutput, statusErr := statusCmd.Output()
		if c.log != nil {
			c.log.LogCmd(statusCmdStr, statusOutput, statusErr)
			c.log.LogEvent("Connection failed: SSID=%s", ssid)
		}
		c.diagnoseConnectionFailure(statusOutput, ssid, password)
	} else {
		fmt.Println("\n" + i18n.T("conn.connected_no_ip"))
		fmt.Println(i18n.Tf("conn.connected_to", connSSID))
		fmt.Println(i18n.T("conn.manual_dhcp_hint"))
		if c.log != nil {
			c.log.LogEvent("Connected but no IP: SSID=%s", connSSID)
		}
	}

	return nil
}

// Stop stops the wpa_supplicant daemon, cleans up network interfaces, DHCP clients and IPs
func (c *Connector) Stop() error {
	iface := c.module.Kernel.GetInterface()

	// 1. Graceful disconnect and terminate via wpa_cli if available
	if c.wpaCliPath != "" && c.ctrlInterface != "" {
		exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "disconnect").Run()
		exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "terminate").Run()
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Kill wpa_supplicant processes via both killall and pkill (-9)
	for _, proc := range []string{"wpa_supplicant", "wpa_supplicant_s1g"} {
		exec.Command("killall", "-9", proc).Run()
		exec.Command("pkill", "-9", proc).Run()
	}

	// 3. Kill DHCP clients (udhcpc, dhclient)
	for _, dhcp := range []string{"udhcpc", "dhclient"} {
		exec.Command("killall", "-9", dhcp).Run()
		exec.Command("pkill", "-9", dhcp).Run()
	}

	// 4. Flush IP address on interface and reset link state
	c.forceCleanup()

	// 5. Cleanup socket files and logs
	c.cleanupSocketFiles()
	if c.wpaLog != nil {
		c.wpaLog.Close()
		c.wpaLog = nil
	}

	c.daemonStarted = false
	fmt.Println(i18n.T("conn.wpa_stopped"))
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
			fmt.Println(i18n.Tf("conn.cleanup_socket", socketPath))
			if err := os.Remove(socketPath); err != nil {
				return fmt.Errorf(i18n.T("conn.cleanup_socket_err"), socketPath, err)
			}
		}
	}

	return nil
}

// runDHCPClient runs DHCP client with timeout protection
func (c *Connector) runDHCPClient() error {
	iface := c.module.Kernel.GetInterface()

	// 1. Try udhcpc with 8-second timeout
	udhcpcPath := sys.FindCommand("udhcpc", "/sbin/udhcpc")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel1()

	args := []string{"-i", iface, "-n", "-q", "-t", "3", "-T", "2"}
	cmdStr := fmt.Sprintf("%s -i %s -n -q -t 3 -T 2", udhcpcPath, iface)
	fmt.Printf("[CMD] %s\n", cmdStr)

	cmd := exec.CommandContext(ctx1, udhcpcPath, args...)
	output, err := cmd.CombinedOutput()
	if c.log != nil {
		c.log.LogCmd(cmdStr, output, err)
	}
	if err == nil {
		fmt.Println(i18n.T("conn.ip_obtained"))
		return nil
	}

	// 2. Try dhclient with 8-second timeout
	dhclientPath := sys.FindCommand("dhclient", "/sbin/dhclient")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel2()

	dhArgs := []string{"-1", "-v", iface}
	dhCmdStr := fmt.Sprintf("%s -1 -v %s", dhclientPath, iface)
	fmt.Printf("[CMD] %s\n", dhCmdStr)

	cmd = exec.CommandContext(ctx2, dhclientPath, dhArgs...)
	output, err = cmd.CombinedOutput()
	if c.log != nil {
		c.log.LogCmd(dhCmdStr, output, err)
	}
	if err == nil {
		fmt.Println(i18n.T("conn.ip_obtained"))
		return nil
	}

	return fmt.Errorf(i18n.T("conn.dhcp_all_fail"))
}

// diagnoseConnectionFailure analyzes why connection failed
func (c *Connector) diagnoseConnectionFailure(statusOutput []byte, ssid, password string) {
	iface := c.module.Kernel.GetInterface()
	statusStr := string(statusOutput)

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("conn.diag_title") + " ==========" + ui.Reset)

	fmt.Println(i18n.T("conn.diag_status"))
	lines := strings.Split(statusStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "wpa_state=") ||
			strings.HasPrefix(line, "ssid=") ||
			strings.HasPrefix(line, "key_mgmt=") {
			fmt.Printf("  %s\n", line)
		}
	}

	fmt.Println("\n" + i18n.T("conn.diag_reasons"))
	reasons := []string{}

	if strings.Contains(statusStr, "wpa_state=SCANNING") {
		// If it's stuck in SCANNING, it usually means it can't find the AP on the allowed frequencies/signal
		reasons = append(reasons, i18n.T("conn.diag_ap_not_found"))
	} else if strings.Contains(statusStr, "wpa_state=4WAY_HANDSHAKE") || strings.Contains(statusStr, "wpa_state=DISCONNECTED") {
		// Only suspect password or auth issues if we moved past scanning
		if password != "" {
			reasons = append(reasons, i18n.T("conn.diag_pwd_wrong"))
			reasons = append(reasons, i18n.T("conn.diag_auth_timeout"))
		} else {
			reasons = append(reasons, i18n.T("conn.diag_need_pwd"))
		}
	} else {
		// Fallback for other states
		if password != "" {
			reasons = append(reasons, i18n.T("conn.diag_pwd_wrong"))
		}
	}

	// Only check signal strength if we're not stuck in SCANNING (RSSI is meaningless in that state)
	if !strings.Contains(statusStr, "wpa_state=SCANNING") {
		cmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "signal_poll")
		if output, err := cmd.Output(); err == nil {
			if strings.Contains(string(output), "RSSI=") {
				reasons = append(reasons, i18n.T("conn.diag_signal_weak"))
			}
		}
	}

	if len(reasons) == 0 {
		fmt.Println(i18n.T("conn.diag_unknown"))
	} else {
		for i, reason := range reasons {
			fmt.Printf("  %d. %s\n", i+1, reason)
		}
	}
	fmt.Println(ui.Blue + ui.Bold + "==============================" + ui.Reset)
}
