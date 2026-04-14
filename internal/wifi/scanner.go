package wifi

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ScanResult represents a single WiFi network
type ScanResult struct {
	BSSID     string
	Frequency string
	Signal    string
	SSID      string
	Flags     string // Security and capability flags
}

type Scanner struct {
	wpaCliPath string
	socketPath string
	iface      string
}

// NewScanner creates a new scanner instance
func NewScanner(wpaCliPath, socketPath, iface string) *Scanner {
	return &Scanner{
		wpaCliPath: wpaCliPath,
		socketPath: socketPath,
		iface:      iface,
	}
}

// Scan performs a WiFi scan and returns the results
func (s *Scanner) Scan() ([]ScanResult, error) {
	// Trigger scan
	if err := s.execWpaCli("scan"); err != nil {
		return nil, fmt.Errorf("failed to trigger scan: %w", err)
	}

	fmt.Println("[INFO] Scanning for networks (please wait 5 seconds)...")
	// Wait for scan to complete using time.Sleep (more portable than exec sleep)
	time.Sleep(5 * time.Second)

	// Get scan results
	output, err := s.execWpaCliOutput("scan_results")
	if err != nil {
		return nil, fmt.Errorf("failed to get scan results: %w", err)
	}

	return s.parseScanResults(output), nil
}

// execWpaCli executes a wpa_cli command
func (s *Scanner) execWpaCli(command string) error {
	args := []string{"-p", s.socketPath, "-i", s.iface, command}
	cmd := exec.Command(s.wpaCliPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}

	return nil
}

// execWpaCliOutput executes a wpa_cli command and returns output
func (s *Scanner) execWpaCliOutput(command string) (string, error) {
	args := []string{"-p", s.socketPath, "-i", s.iface, command}
	cmd := exec.Command(s.wpaCliPath, args...)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// parseScanResults parses wpa_cli scan_results output
func (s *Scanner) parseScanResults(output string) []ScanResult {
	var results []ScanResult

	lines := strings.Split(output, "\n")

	// Skip header line
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		// Format: bssid / frequency / signal level / flags / ssid
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// SSID is the last field (may contain spaces)
		ssidStart := strings.Index(line, fields[4])
		ssid := strings.TrimSpace(line[ssidStart:])

		results = append(results, ScanResult{
			BSSID:     fields[0],
			Frequency: fields[1],
			Signal:    fields[2],
			SSID:      ssid,
			Flags:     fields[3], // Store flags for security info
		})
	}

	return results
}

// PrintResults displays scan results in a formatted list
func PrintScanResults(results []ScanResult) {
	fmt.Println("\n========== 掃描結果 ==========")

	if len(results) == 0 {
		fmt.Println("未找到任何網路")
		return
	}

	for i, result := range results {
		// Parse band from frequency
		band := parseBand(result.Frequency)

		// Parse security from flags
		security := parseSecurity(result.Flags)

		// Parse signal strength quality
		signalQuality := parseSignalQuality(result.Signal)

		// Parse WiFi generation
		wifiGen := parseWiFiGeneration(result.Frequency, result.Flags)

		// Truncate SSID if too long
		ssid := result.SSID
		if len(ssid) > 17 {
			ssid = ssid[:16] + "..."
		}

		// Format: [1] SSID: Mozart-AP8   協議: WiFi 5   頻段: 5GHz   安全性: WPA2-Personal   信號: -36 dBm (良好)
		fmt.Printf("[%d] SSID: %-17s\t協議: %-8s\t頻段: %-8s\t安全性: %-18s\t信號: %s dBm (%s)\n",
			i+1,
			ssid,
			wifiGen,
			band,
			security,
			result.Signal,
			signalQuality,
		)
	}

	fmt.Println("==============================")
}

// parseBand determines WiFi band from frequency
func parseBand(freqStr string) string {
	freq := 0
	fmt.Sscanf(freqStr, "%d", &freq)

	if freq >= 700 && freq <= 1000 {
		return "sub-GHz"
	} else if freq >= 2400 && freq <= 2500 {
		return "2.4GHz"
	} else if freq >= 5150 && freq <= 5895 {
		return "5GHz"
	} else if freq >= 5955 && freq <= 7115 {
		return "6GHz"
	}
	return fmt.Sprintf("%d MHz", freq)
}

// parseSecurity determines security type from flags
func parseSecurity(flags string) string {
	flags = strings.ToUpper(flags)

	if strings.Contains(flags, "SAE") || strings.Contains(flags, "WPA3") {
		return "WPA3-SAE"
	} else if strings.Contains(flags, "WPA2-PSK") || strings.Contains(flags, "RSN-PSK") {
		return "WPA2-Personal"
	} else if strings.Contains(flags, "WPA-PSK") {
		return "WPA-Personal"
	} else if strings.Contains(flags, "WPA2") && strings.Contains(flags, "WPA") {
		return "WPA/WPA2-Personal"
	} else if strings.Contains(flags, "WEP") {
		return "WEP"
	} else if strings.Contains(flags, "ESS") && !strings.Contains(flags, "WPA") {
		return "開放網路"
	}

	return "Unknown"
}

// parseSignalQuality determines signal quality from signal strength (dBm)
func parseSignalQuality(signalStr string) string {
	signal := 0
	fmt.Sscanf(signalStr, "%d", &signal)

	if signal >= -50 {
		return "優秀"
	} else if signal >= -60 {
		return "良好"
	} else if signal >= -70 {
		return "中等"
	} else if signal >= -80 {
		return "較差"
	}
	return "很差"
}

// parseWiFiGeneration determines WiFi generation from frequency and flags
func parseWiFiGeneration(freqStr, flags string) string {
	freq := 0
	fmt.Sscanf(freqStr, "%d", &freq)
	flagsUpper := strings.ToUpper(flags)

	// WiFi HaLow (802.11ah) - check frequencies or flags
	if (freq >= 700 && freq <= 1000) || strings.Contains(flagsUpper, "AH") {
		return "HaLow"
	}

	// WiFi 7 (802.11be) - check for EHT/BE capabilities
	if strings.Contains(flagsUpper, "EHT") || strings.Contains(flagsUpper, "BE") {
		return "WiFi 7"
	}

	// WiFi 6E (802.11ax on 6GHz)
	if freq >= 5955 && freq <= 7115 {
		return "WiFi 6E"
	}

	// WiFi 6 (802.11ax) - check for HE capabilities
	if strings.Contains(flagsUpper, "HE") || strings.Contains(flagsUpper, "AX") {
		return "WiFi 6"
	}

	// WiFi 5 (802.11ac) - 5GHz with VHT
	if strings.Contains(flagsUpper, "VHT") || strings.Contains(flagsUpper, "AC") {
		return "WiFi 5"
	}

	// WiFi 5 (802.11ac) - 5GHz band
	if freq >= 5150 && freq <= 5895 {
		return "WiFi 5"
	}

	// WiFi 4 (802.11n) - HT capabilities or 2.4GHz
	if strings.Contains(flagsUpper, "HT") {
		return "WiFi 4"
	}

	// Default to WiFi 4 for 2.4GHz
	if freq >= 2400 && freq <= 2500 {
		return "WiFi 4"
	}

	return "WiFi 4"
}
