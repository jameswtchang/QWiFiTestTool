package wifi

import (
	"fmt"
	"os/exec"
	"strings"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/ui"
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
	iface := c.module.Kernel.GetInterface()

	// 1. Try modern 'iw dev <iface> link'
	if linkOut, err := exec.Command("iw", "dev", iface, "link").Output(); err == nil {
		outStr := string(linkOut)
		if strings.Contains(outStr, "Connected to ") {
			for _, line := range strings.Split(outStr, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "SSID: ") {
					ssid := strings.TrimPrefix(line, "SSID: ")
					if ssid != "" {
						return true, ssid
					}
				}
			}
			return true, "Connected"
		}
		if strings.Contains(outStr, "Not connected.") {
			return false, ""
		}
	}

	// 2. Fallback to iwconfig
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

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("conn.info_title") + " ==========" + ui.Reset)

	// Wireless info via iw dev link or iwconfig
	var ssid, bssid, freqStr, signalStr, txBitrate, rxBitrate string
	if linkOut, err := exec.Command("iw", "dev", iface, "link").Output(); err == nil {
		for _, line := range strings.Split(string(linkOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "SSID: ") {
				ssid = strings.TrimPrefix(line, "SSID: ")
			} else if strings.HasPrefix(line, "Connected to ") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					bssid = parts[2]
				}
			} else if strings.HasPrefix(line, "freq: ") {
				freqStr = strings.TrimPrefix(line, "freq: ")
			} else if strings.HasPrefix(line, "signal: ") {
				signalStr = strings.TrimPrefix(line, "signal: ")
			} else if strings.HasPrefix(line, "tx bitrate: ") {
				txBitrate = strings.TrimPrefix(line, "tx bitrate: ")
			} else if strings.HasPrefix(line, "rx bitrate: ") {
				rxBitrate = strings.TrimPrefix(line, "rx bitrate: ")
			}
		}
	}

	// Fallback to iwconfig if needed
	if ssid == "" {
		if iwOut, err := exec.Command("iwconfig", iface).Output(); err == nil {
			for _, line := range strings.Split(string(iwOut), "\n") {
				if strings.Contains(line, "ESSID:") {
					parts := strings.Split(line, "ESSID:")
					if len(parts) >= 2 {
						ssid = strings.Trim(strings.TrimSpace(parts[1]), "\"")
					}
				}
				if strings.Contains(line, "Access Point:") {
					parts := strings.Split(line, "Access Point:")
					if len(parts) >= 2 {
						bssid = strings.Fields(parts[1])[0]
					}
				}
			}
		}
	}

	// Channel info via iw dev info
	var iwChannel string
	if iwOut, err := exec.Command("iw", "dev", iface, "info").Output(); err == nil {
		for _, line := range strings.Split(string(iwOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "channel ") {
				iwChannel = line
			}
		}
	}

	chDisplay := c.resolveChannelDisplay(iface, iwChannel, freqStr)
	bitrateDisplay := c.resolveBitrateDisplay(iface, txBitrate, rxBitrate)

	// IP info
	var ipWithCidr, gatewayIP string
	if ipOut, err := exec.Command("ip", "-4", "addr", "show", iface).Output(); err == nil {
		for _, line := range strings.Split(string(ipOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "inet ") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					ipWithCidr = fields[1]
					break
				}
			}
		}
	}
	if routeOut, err := exec.Command("ip", "route", "show", "dev", iface).Output(); err == nil {
		for _, line := range strings.Split(string(routeOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "default via ") {
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					gatewayIP = fields[2]
					break
				}
			}
		}
	}

	// ==========================================
	// Presentation / Printing
	// ==========================================

	// Group 1: Basic Status & Auth
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.iface"), 16), iface)
	if ssid != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Cyan+"%s"+ui.Reset+"\n", ui.AlignLabel("SSID:", 16), ssid)
	}
	if bssid != "" && bssid != "Not-Associated" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("BSSID:", 16), bssid)
	}

	// Group 2: RF & Link Quality
	fmt.Println()
	if chDisplay != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.band_channel"), 16), chDisplay)
	}
	if bitrateDisplay != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.bitrate"), 16), bitrateDisplay)
	}
	if signalStr != "" {
		var signalDbm int
		fmt.Sscanf(signalStr, "%d", &signalDbm)
		quality := parseSignalQuality(fmt.Sprintf("%d", signalDbm))
		sigColor := ui.Green
		if signalDbm < -75 {
			sigColor = ui.Red
		} else if signalDbm < -65 {
			sigColor = ui.Yellow
		}
		fmt.Printf(ui.Bold+"  %s "+sigColor+"%d dBm (%s)"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.signal"), 16), signalDbm, quality)
	}

	// Group 3: Network Layer (IP/Gateway)
	if ipWithCidr != "" || gatewayIP != "" {
		fmt.Println()
		if ipWithCidr != "" {
			fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.ip"), 16), ipWithCidr)
		}
		if gatewayIP != "" {
			fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.gateway"), 16), gatewayIP)
		}
	}

	fmt.Println(ui.Blue + ui.Bold + "==============================" + ui.Reset)
}
