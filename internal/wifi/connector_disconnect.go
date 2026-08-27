package wifi

import (
	"fmt"
	"os/exec"
	"time"

	"qwifitesttool/internal/i18n"
)

// stopWpaSupplicant stops wpa_supplicant daemon with command logging
func (c *Connector) stopWpaSupplicant() {
	iface := c.module.Kernel.GetInterface()

	fmt.Println("\n" + i18n.T("conn.stop_wpa"))
	if c.log != nil {
		c.log.LogEvent("Stopping wpa_supplicant daemon")
	}

	// Try graceful stop via wpa_cli
	terminateCmd := fmt.Sprintf("%s -p %s -i %s terminate", c.wpaCliPath, c.ctrlInterface, iface)
	fmt.Printf("[CMD] %s\n", terminateCmd)

	cmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "terminate")
	output, err := cmd.CombinedOutput()
	if c.log != nil {
		c.log.LogCmd(terminateCmd, output, err)
	}
	if err != nil {
		fmt.Println(i18n.Tf("conn.wpa_cli_fail", err))
	}

	time.Sleep(1 * time.Second)

	// Force kill if still running
	for _, proc := range []string{"wpa_supplicant", "wpa_supplicant_s1g"} {
		killCmd := fmt.Sprintf("pkill -9 %s", proc)
		fmt.Printf("[CMD] %s\n", killCmd)
		killOutput, killErr := exec.Command("pkill", "-9", proc).CombinedOutput()
		if c.log != nil {
			c.log.LogCmd(killCmd, killOutput, killErr)
		}
	}

	c.daemonStarted = false
	if c.wpaLog != nil {
		c.wpaLog.Close()
		c.wpaLog = nil
	}
	fmt.Println(i18n.T("conn.wpa_stopped2"))
}

// forceCleanup performs force cleanup of network interface
func (c *Connector) forceCleanup() {
	iface := c.module.Kernel.GetInterface()

	fmt.Println(i18n.T("conn.force_cleanup"))

	// Flush IP
	flushCmd := fmt.Sprintf("ip addr flush dev %s", iface)
	fmt.Printf("[CMD] %s\n", flushCmd)
	exec.Command("ip", "addr", "flush", "dev", iface).Run()

	// Bring interface down and up
	downCmd := fmt.Sprintf("ip link set %s down", iface)
	fmt.Printf("[CMD] %s\n", downCmd)
	exec.Command("ip", "link", "set", iface, "down").Run()

	time.Sleep(500 * time.Millisecond)

	upCmd := fmt.Sprintf("ip link set %s up", iface)
	fmt.Printf("[CMD] %s\n", upCmd)
	exec.Command("ip", "link", "set", iface, "up").Run()
}

// autoConnectFromConfig attempts to connect using existing config file
func (c *Connector) autoConnectFromConfig() (bool, error) {
	fmt.Println(i18n.Tf("conn.using_config", c.configPath))

	// Start daemon
	if err := c.startDaemon(); err != nil {
		return false, err
	}

	// Wait for auto-connect
	fmt.Println(i18n.T("conn.wait_auto"))
	time.Sleep(8 * time.Second)

	// Check connection status
	connected, ssid := c.checkConnectionStatus()
	if !connected {
		fmt.Println(i18n.T("conn.auto_fail2"))
		fmt.Println(i18n.T("conn.auto_fail_reasons"))
		fmt.Println(i18n.T("conn.reason_pwd"))
		fmt.Println(i18n.T("conn.reason_range"))
		fmt.Println(i18n.T("conn.reason_reject"))

		fmt.Print(i18n.T("conn.ask_scan"))
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			return c.interactiveScanConnect()
		}

		return false, nil
	}

	fmt.Printf("%s\n", i18n.Tf("conn.connected_to2", ssid))

	// Check IP and get if needed
	hasIP, ipAddr := c.checkIPAddress()
	if hasIP {
		fmt.Printf("%s\n", i18n.Tf("conn.ip_addr2", ipAddr))
	} else {
		fmt.Println(i18n.T("conn.getting_ip"))
		if err := c.runDHCPClient(); err != nil {
			fmt.Println(i18n.Tf("conn.dhcp_fail", err))
		} else {
			time.Sleep(2 * time.Second)
			hasIP, ipAddr = c.checkIPAddress()
			if hasIP {
				fmt.Printf("%s\n", i18n.Tf("conn.ip_addr2", ipAddr))
			}
		}
	}

	c.showConnectionInfo()
	return false, nil
}
