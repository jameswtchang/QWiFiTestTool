package wifi

import (
	"fmt"
	"os/exec"
	"strings"
)

// checkIPAddressDirect checks IP without requiring wpa_supplicant to be running
func (c *Connector) checkIPAddressDirect() (bool, string) {
	iface := c.module.Kernel.GetInterface()

	cmd := exec.Command("ip", "addr", "show", iface)
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ipWithMask := fields[1]
				ip := strings.Split(ipWithMask, "/")[0]
				return true, ip
			}
		}
	}

	return false, ""
}

// checkConnectionStatusDirect tries to get connection status without requiring running wpa_supplicant
func (c *Connector) checkConnectionStatusDirect() (bool, string) {
	// Try to read /proc/net/wireless to see if connected
	iface := c.module.Kernel.GetInterface()

	cmd := exec.Command("iwconfig", iface)
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	outputStr := string(output)

	// Look for ESSID
	if strings.Contains(outputStr, "ESSID:") {
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "ESSID:") {
				// Extract ESSID
				start := strings.Index(line, "ESSID:\"")
				if start != -1 {
					start += 7
					end := strings.Index(line[start:], "\"")
					if end != -1 {
						ssid := line[start : start+end]
						if ssid != "" && ssid != "off/any" {
							return true, ssid
						}
					}
				}
			}
		}
	}

	return false, ""
}

// showConnectionInfoDirect shows connection info without wpa_cli
func (c *Connector) showConnectionInfoDirect() {
	iface := c.module.Kernel.GetInterface()

	fmt.Println("\n========== 連線資訊 ==========")

	// Get IP info
	cmd := exec.Command("ip", "addr", "show", iface)
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "inet ") || strings.HasPrefix(line, "inet6 ") {
				fmt.Printf("  %s\n", line)
			}
		}
	}

	// Get wireless info
	cmd = exec.Command("iwconfig", iface)
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "ESSID:") ||
				strings.Contains(line, "Mode:") ||
				strings.Contains(line, "Frequency:") ||
				strings.Contains(line, "Access Point:") {
				fmt.Printf("  %s\n", strings.TrimSpace(line))
			}
		}
	}

	fmt.Println("==============================")
}
