package wifi

import (
	"bytes"
	"fmt"
	"os/exec"
	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/logger"
	"qwifitesttool/internal/ui"
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
	log        *logger.SessionLogger
}

// NewScanner creates a new scanner instance
func NewScanner(wpaCliPath, socketPath, iface string) *Scanner {
	return &Scanner{
		wpaCliPath: wpaCliPath,
		socketPath: socketPath,
		iface:      iface,
		log:        logger.GetCurrentSession(),
	}
}

// Scan performs a WiFi scan and returns the results
func (s *Scanner) Scan() ([]ScanResult, error) {
	// Trigger scan
	if err := s.execWpaCli("scan"); err != nil {
		return nil, fmt.Errorf("failed to trigger scan: %w", err)
	}

	fmt.Println(i18n.T("scan.scanning"))
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
	cmdStr := fmt.Sprintf("%s %s", s.wpaCliPath, strings.Join(args, " "))
	cmd := exec.Command(s.wpaCliPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if s.log != nil {
		combined := output
		if stderr.Len() > 0 {
			combined = append(combined, stderr.Bytes()...)
		}
		s.log.LogCmd(cmdStr, combined, err)
	}

	if err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}

	return nil
}

// execWpaCliOutput executes a wpa_cli command and returns output
func (s *Scanner) execWpaCliOutput(command string) (string, error) {
	args := []string{"-p", s.socketPath, "-i", s.iface, command}
	cmdStr := fmt.Sprintf("%s %s", s.wpaCliPath, strings.Join(args, " "))
	cmd := exec.Command(s.wpaCliPath, args...)

	output, err := cmd.Output()
	if s.log != nil {
		s.log.LogCmd(cmdStr, output, err)
	}
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

		// Format: bssid \t frequency \t signal level \t flags \t ssid
		// Fields are tab-separated; the SSID may contain spaces and is printf-escaped
		// by wpa_cli (\xNN for UTF-8 / non-printable bytes), so decode it to raw bytes.
		line = strings.TrimRight(line, "\r")
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 5 {
			// Fallback for non-tab output (legacy behaviour): whitespace-split,
			// SSID is the remainder after the 4th field
			ws := strings.Fields(line)
			if len(ws) < 5 {
				continue
			}
			ssidStart := strings.Index(line, ws[4])
			fields = []string{ws[0], ws[1], ws[2], ws[3], strings.TrimSpace(line[ssidStart:])}
		}

		ssid := decodeWpaString(fields[4])
		if ssid == "" {
			continue // Hidden network
		}

		results = append(results, ScanResult{
			BSSID:     strings.TrimSpace(fields[0]),
			Frequency: strings.TrimSpace(fields[1]),
			Signal:    strings.TrimSpace(fields[2]),
			SSID:      ssid,
			Flags:     strings.TrimSpace(fields[3]), // Store flags for security info
		})
	}

	return results
}

// PrintResults displays scan results in a formatted list
func PrintScanResults(results []ScanResult) {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("scan.result_title") + " ==========" + ui.Reset)

	if len(results) == 0 {
		fmt.Println(i18n.T("scan.no_network"))
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

		// Format: [1] SSID: Mozart-AP8   Protocol: WiFi 5   Band: 5GHz   Security: WPA2-Personal   Signal: -36 dBm (Good)
		fmt.Printf("[%d] SSID: %-17s\t%s: %-8s\t%s: %-8s\t%s: %-18s\t%s: %s dBm (%s)\n",
			i+1,
			ssid,
			i18n.T("scan.col.protocol"),
			wifiGen,
			i18n.T("scan.col.band"),
			band,
			i18n.T("scan.col.security"),
			security,
			i18n.T("scan.col.signal"),
			result.Signal,
			signalQuality,
		)
	}

	fmt.Println(ui.Blue + ui.Bold + "==============================" + ui.Reset)
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
		return i18n.T("scan.open_network")
	}

	return "Unknown"
}

// parseSignalQuality determines signal quality from signal strength (dBm)
func parseSignalQuality(signalStr string) string {
	signal := 0
	fmt.Sscanf(signalStr, "%d", &signal)

	if signal >= -50 {
		return i18n.T("scan.signal.excellent")
	} else if signal >= -60 {
		return i18n.T("scan.signal.good")
	} else if signal >= -70 {
		return i18n.T("scan.signal.fair")
	} else if signal >= -80 {
		return i18n.T("scan.signal.poor")
	}
	return i18n.T("scan.signal.very_poor")
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
