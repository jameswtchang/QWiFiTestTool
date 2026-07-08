package wifi

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"qwifitesttool/internal/ui"
)

// checkConnectionStatus checks if currently connected to a network
func (c *Connector) checkConnectionStatus() (bool, string) {
	iface := c.module.Kernel.GetInterface()

	cmdStr := fmt.Sprintf("%s -p %s -i %s status", c.wpaCliPath, c.ctrlInterface, iface)
	cmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "status")
	output, err := cmd.Output()
	if c.log != nil {
		c.log.LogCmd(cmdStr, output, err)
	}
	if err != nil {
		return false, ""
	}

	statusStr := string(output)

	// Check if wpa_state is COMPLETED
	if !strings.Contains(statusStr, "wpa_state=COMPLETED") {
		return false, ""
	}

	// Extract SSID
	lines := strings.Split(statusStr, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ssid=") {
			ssid := strings.TrimPrefix(line, "ssid=")
			return true, strings.TrimSpace(ssid)
		}
	}

	return true, "Unknown"
}

// disconnect disconnects from current network
func (c *Connector) disconnect() error {
	iface := c.module.Kernel.GetInterface()

	cmdStr := fmt.Sprintf("%s -p %s -i %s disconnect", c.wpaCliPath, c.ctrlInterface, iface)
	cmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "disconnect")
	output, err := cmd.CombinedOutput()
	if c.log != nil {
		c.log.LogCmd(cmdStr, output, err)
	}
	if err != nil {
		return fmt.Errorf("斷開連線失敗: %w", err)
	}

	// Give it a moment to disconnect
	time.Sleep(1 * time.Second)
	return nil
}

// checkIPAddress checks if the interface has an IP address
func (c *Connector) checkIPAddress() (bool, string) {
	iface := c.module.Kernel.GetInterface()

	// Use 'ip addr show' command
	cmd := exec.Command("ip", "addr", "show", iface)
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	outputStr := string(output)

	// Look for 'inet ' followed by IP address
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			// Extract IP address
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// Remove /24 or similar subnet mask
				ipWithMask := fields[1]
				ip := strings.Split(ipWithMask, "/")[0]
				return true, ip
			}
		}
	}

	return false, ""
}

// showConnectionInfo displays detailed connection information
func (c *Connector) showConnectionInfo() {
	iface := c.module.Kernel.GetInterface()

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== 連線資訊 ==========" + ui.Reset)

	// Get wpa_supplicant status
	cmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "status")
	output, err := cmd.Output()
	if err == nil {
		statusStr := string(output)
		lines := strings.Split(statusStr, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Print relevant fields
			if strings.HasPrefix(line, "ssid=") ||
				strings.HasPrefix(line, "bssid=") ||
				strings.HasPrefix(line, "freq=") ||
				strings.HasPrefix(line, "ip_address=") ||
				strings.HasPrefix(line, "wpa_state=") ||
				strings.HasPrefix(line, "pairwise_cipher=") ||
				strings.HasPrefix(line, "group_cipher=") ||
				strings.HasPrefix(line, "key_mgmt=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					fmt.Printf(ui.Bold+"  %-16s "+ui.Green+"%s"+ui.Reset+"\n", parts[0]+":", parts[1])
				} else {
					fmt.Printf(ui.Green+"  %s"+ui.Reset+"\n", line)
				}
			}
		}
	}

	// Get IP info from ifconfig/ip
	cmd = exec.Command("ip", "addr", "show", iface)
	if output, err := cmd.Output(); err == nil {
		fmt.Println("\n" + ui.Cyan + ui.Bold + "網路介面資訊:" + ui.Reset)
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "inet ") || strings.HasPrefix(line, "inet6 ") {
				fmt.Printf("  %s\n", line)
			}
		}
		// Show iw interface details
		ShowInterfaceDetail(iface)
	}

	fmt.Println(ui.Blue + ui.Bold + "==============================" + ui.Reset)
}
