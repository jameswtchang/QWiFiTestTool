package wifi

import (
	"fmt"
	"os/exec"
	"time"
)

// stopWpaSupplicant stops wpa_supplicant daemon with command logging
func (c *Connector) stopWpaSupplicant() {
	iface := c.module.Kernel.GetInterface()

	fmt.Println("\n[INFO] 停止 wpa_supplicant daemon...")

	// Try graceful stop via wpa_cli
	terminateCmd := fmt.Sprintf("%s -p %s -i %s terminate", c.wpaCliPath, c.ctrlInterface, iface)
	fmt.Printf("[CMD] %s\n", terminateCmd)

	cmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "terminate")
	if err := cmd.Run(); err != nil {
		fmt.Printf("[WARN] wpa_cli terminate 失敗: %v\n", err)
	}

	time.Sleep(1 * time.Second)

	// Force kill if still running
	for _, proc := range []string{"wpa_supplicant", "wpa_supplicant_s1g"} {
		fmt.Printf("[CMD] pkill -9 %s\n", proc)
		exec.Command("pkill", "-9", proc).Run()
	}

	c.daemonStarted = false
	fmt.Println("[INFO] wpa_supplicant 已停止")
}

// forceCleanup performs force cleanup of network interface
func (c *Connector) forceCleanup() {
	iface := c.module.Kernel.GetInterface()

	fmt.Println("[INFO] 強制清理網路介面...")

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
	fmt.Printf("[INFO] 使用配置文件: %s\n", c.configPath)

	// Start daemon
	if err := c.startDaemon(); err != nil {
		return false, err
	}

	// Wait for auto-connect
	fmt.Println("[INFO] 等待自動連接...")
	time.Sleep(8 * time.Second)

	// Check connection status
	connected, ssid := c.checkConnectionStatus()
	if !connected {
		fmt.Println("[WARN] 自動連接失敗")
		fmt.Println("[INFO] 可能原因:")
		fmt.Println("  - 配置文件中的密碼錯誤")
		fmt.Println("  - AP 不在範圍內")
		fmt.Println("  - AP 拒絕連接")

		fmt.Print("\n是否進入掃描模式? [y/N]: ")
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			return c.interactiveScanConnect()
		}

		return false, nil
	}

	fmt.Printf("✓ 已連接到: %s\n", ssid)

	// Check IP and get if needed
	hasIP, ipAddr := c.checkIPAddress()
	if hasIP {
		fmt.Printf("✓ IP 地址: %s\n", ipAddr)
	} else {
		fmt.Println("[INFO] 獲取 IP 地址...")
		if err := c.runDHCPClient(); err != nil {
			fmt.Printf("[WARN] DHCP 失敗: %v\n", err)
		} else {
			time.Sleep(2 * time.Second)
			hasIP, ipAddr = c.checkIPAddress()
			if hasIP {
				fmt.Printf("✓ IP 地址: %s\n", ipAddr)
			}
		}
	}

	c.showConnectionInfo()
	return false, nil
}
