package wifi

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"qwifitesttool/internal/i18n"
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
		return fmt.Errorf(i18n.T("conn.disconnect_fail"), err)
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

	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("conn.info_title") + " ==========" + ui.Reset)

	// 1. Parse wpa_supplicant status
	statusMap := make(map[string]string)
	cmd := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "status")
	if output, err := cmd.Output(); err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				statusMap[parts[0]] = parts[1]
			}
		}
	}

	// 2. Interface
	fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.iface"), 16), iface)

	// 3. SSID
	ssid := statusMap["ssid"]
	if ssid != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Cyan+"%s"+ui.Reset+"\n", ui.AlignLabel("SSID:", 16), ssid)
	}

	// 4. BSSID
	if bssid := statusMap["bssid"]; bssid != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("BSSID:", 16), bssid)
	}

	// 5. Connection State
	if state := statusMap["wpa_state"]; state != "" {
		stateDisplay := state
		stateColor := ui.Yellow
		switch state {
		case "COMPLETED":
			stateDisplay = i18n.T("conn.status_completed")
			stateColor = ui.Green
		case "DISCONNECTED":
			stateDisplay = i18n.T("conn.status_disconnected")
			stateColor = ui.Red
		case "SCANNING":
			stateDisplay = i18n.T("conn.status_scanning")
		case "ASSOCIATING":
			stateDisplay = i18n.T("conn.status_associating")
		case "4WAY_HANDSHAKE":
			stateDisplay = i18n.T("conn.status_handshake")
		}
		fmt.Printf(ui.Bold+"  %s "+stateColor+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.status"), 16), stateDisplay)
	}

	// 6. Channel / Band info & TxPower from 'iw dev <iface> info'
	var iwChannel, txPower string
	if iwOut, err := exec.Command("iw", "dev", iface, "info").Output(); err == nil {
		for _, line := range strings.Split(string(iwOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "channel ") {
				iwChannel = line
			} else if strings.HasPrefix(line, "txpower ") {
				txPower = FormatTxPower(line)
			}
		}
	}

	chDisplay := formatChannelInfo(iwChannel, statusMap["freq"])
	if chDisplay != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.band_channel"), 16), chDisplay)
	}

	// 7. Signal & Bitrate from 'iw dev <iface> link' (fallback to wpa_cli signal_poll)
	var signalStr, txBitrate, rxBitrate string
	if linkOut, err := exec.Command("iw", "dev", iface, "link").Output(); err == nil {
		for _, line := range strings.Split(string(linkOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "signal: ") {
				signalStr = strings.TrimPrefix(line, "signal: ")
			} else if strings.HasPrefix(line, "tx bitrate: ") {
				txBitrate = strings.TrimPrefix(line, "tx bitrate: ")
			} else if strings.HasPrefix(line, "rx bitrate: ") {
				rxBitrate = strings.TrimPrefix(line, "rx bitrate: ")
			}
		}
	}

	if signalStr == "" || (txBitrate == "" && rxBitrate == "") {
		if pollOut, err := exec.Command(c.wpaCliPath, "-p", c.ctrlInterface, "-i", iface, "signal_poll").Output(); err == nil {
			for _, line := range strings.Split(string(pollOut), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "RSSI=") && signalStr == "" {
					signalStr = strings.TrimPrefix(line, "RSSI=") + " dBm"
				} else if strings.HasPrefix(line, "LINKSPEED=") && txBitrate == "" {
					txBitrate = strings.TrimPrefix(line, "LINKSPEED=") + " Mbps"
				}
			}
		}
	}

	// Print Signal
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

	// Print Bitrate
	bitrateDisplay := formatBitrateDisplay(txBitrate, rxBitrate)
	if bitrateDisplay != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.bitrate"), 16), bitrateDisplay)
	}

	// 8. Security
	keyMgmt := statusMap["key_mgmt"]
	pairwise := statusMap["pairwise_cipher"]
	security := ""
	if strings.Contains(keyMgmt, "SAE") {
		security = "WPA3-SAE"
	} else if strings.Contains(keyMgmt, "WPA2") || keyMgmt == "WPA2-PSK" {
		security = "WPA2-PSK"
	} else if strings.Contains(keyMgmt, "WPA") {
		security = keyMgmt
	} else if keyMgmt != "" && keyMgmt != "NONE" {
		security = keyMgmt
	} else if keyMgmt == "NONE" || (keyMgmt == "" && ssid != "") {
		security = i18n.T("hotspot.open_network")
	}
	if security != "" && pairwise != "" && security != i18n.T("hotspot.open_network") {
		security = fmt.Sprintf("%s (%s)", security, pairwise)
	}
	if security != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.security"), 16), security)
	}

	// 9. IP Address & Default Gateway
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
	if ipWithCidr == "" && statusMap["ip_address"] != "" {
		ipWithCidr = statusMap["ip_address"]
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

	if ipWithCidr != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.ip"), 16), ipWithCidr)
	}
	if gatewayIP != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("conn.label.gateway"), 16), gatewayIP)
	}

	// 10. TxPower
	if txPower != "" {
		fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("iw.txpower"), 16), txPower)
	}

	fmt.Println(ui.Blue + ui.Bold + "==============================" + ui.Reset)
}

// formatChannelInfo combines frequency and iw info into a concise channel/band string
func formatChannelInfo(iwChannel, freqStr string) string {
	freq := 0
	if freqStr != "" {
		fmt.Sscanf(freqStr, "%d", &freq)
	}

	band := ""
	if freq > 0 {
		if freq < 1000 {
			band = "Sub-1GHz"
		} else if freq >= 2400 && freq <= 2500 {
			band = "2.4GHz"
		} else if freq >= 4900 && freq <= 5900 {
			band = "5GHz"
		} else if freq >= 5925 && freq <= 7125 {
			band = "6GHz"
		}
	}

	if iwChannel == "" {
		if freq > 0 {
			ch := calcChannelFromFreq(freq)
			if ch > 0 && band != "" {
				return fmt.Sprintf("%s / CH %d (%d MHz)", band, ch, freq)
			} else if band != "" {
				return fmt.Sprintf("%s (%d MHz)", band, freq)
			}
			return fmt.Sprintf("%d MHz", freq)
		}
		return ""
	}

	// Handle 320MHz width in old iw
	if strings.Contains(iwChannel, "width: unknown") {
		if strings.Contains(iwChannel, "center1: 6105") || strings.Contains(iwChannel, "center1: 6265") || strings.Contains(iwChannel, "center1: 6425") || strings.Contains(iwChannel, "center1: 6585") {
			iwChannel = strings.ReplaceAll(iwChannel, "width: unknown", "width: 320 MHz (802.11be EHT)")
		} else {
			iwChannel = strings.ReplaceAll(iwChannel, "width: unknown", "width: 320 MHz")
		}
	}

	// Example: "channel 157 (5785 MHz), width: 80 MHz, center1: 5775 MHz"
	clean := strings.TrimPrefix(iwChannel, "channel ")
	parts := strings.Split(clean, ", ")
	if len(parts) >= 2 {
		primaryAndFreq := parts[0]
		widthStr := ""
		centerStr := ""
		for _, p := range parts[1:] {
			if strings.HasPrefix(p, "width: ") {
				widthStr = strings.TrimPrefix(p, "width: ")
			} else if strings.HasPrefix(p, "center1: ") {
				centerStr = strings.TrimPrefix(p, "center1: ")
			}
		}

		extra := ""
		if widthStr != "" {
			extra = widthStr
		}
		if centerStr != "" && !strings.Contains(primaryAndFreq, centerStr) {
			if extra != "" {
				extra += ", Center: " + centerStr
			} else {
				extra = "Center: " + centerStr
			}
		}

		if extra != "" {
			if band != "" {
				return fmt.Sprintf("%s / CH %s [%s]", band, primaryAndFreq, extra)
			}
			return fmt.Sprintf("CH %s [%s]", primaryAndFreq, extra)
		}
	}

	if band != "" {
		return fmt.Sprintf("%s / CH %s", band, clean)
	}
	return "CH " + clean
}

// calcChannelFromFreq returns the primary channel number from frequency in MHz
func calcChannelFromFreq(freq int) int {
	if freq >= 2412 && freq <= 2472 {
		return (freq - 2407) / 5
	} else if freq == 2484 {
		return 14
	} else if freq >= 5180 && freq <= 5885 {
		return (freq - 5000) / 5
	} else if freq >= 5955 && freq <= 7115 {
		return (freq - 5950) / 5
	}
	return 0
}

// formatBitrateDisplay formats TX and RX bitrates nicely
func formatBitrateDisplay(txRaw, rxRaw string) string {
	tx := parseBitrate(txRaw)
	rx := parseBitrate(rxRaw)

	if tx != "" && rx != "" {
		if tx == rx {
			return tx
		}
		return fmt.Sprintf("TX: %s / RX: %s", tx, rx)
	} else if tx != "" {
		return tx
	} else if rx != "" {
		return rx
	}
	return ""
}

// parseBitrate extracts clean bitrate and MCS information
func parseBitrate(raw string) string {
	raw = strings.TrimSpace(raw)
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) >= 2 && (strings.HasPrefix(fields[1], "MBit/s") || strings.HasPrefix(fields[1], "GBit/s") || strings.HasPrefix(fields[1], "KBit/s") || strings.HasPrefix(fields[1], "Mbps")) {
		rate := fields[0] + " " + fields[1]
		for i := 2; i < len(fields); i++ {
			if strings.Contains(fields[i], "MCS") {
				mcs := fields[i]
				if i+1 < len(fields) && len(fields[i+1]) > 0 && fields[i+1][0] >= '0' && fields[i+1][0] <= '9' {
					mcs += " " + fields[i+1]
				}
				rate += " (" + mcs + ")"
				break
			}
		}
		return rate
	}
	return raw
}
