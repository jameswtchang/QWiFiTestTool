package wifi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatChannelInfo(t *testing.T) {
	tests := []struct {
		name      string
		iwChannel string
		freqStr   string
		expected  string
	}{
		{
			name:      "5GHz 80MHz with center freq",
			iwChannel: "channel 157 (5785 MHz), width: 80 MHz, center1: 5775 MHz",
			freqStr:   "5785",
			expected:  "5GHz / CH 157 (5785 MHz) [80 MHz, Center: 5775 MHz]",
		},
		{
			name:      "2.4GHz 20MHz center matches primary",
			iwChannel: "channel 6 (2437 MHz), width: 20 MHz, center1: 2437 MHz",
			freqStr:   "2437",
			expected:  "2.4GHz / CH 6 (2437 MHz) [20 MHz]",
		},
		{
			name:      "6GHz 320MHz with unknown width in old iw",
			iwChannel: "channel 37 (6135 MHz), width: unknown, center1: 6265 MHz",
			freqStr:   "6135",
			expected:  "6GHz / CH 37 (6135 MHz) [320 MHz (802.11be EHT), Center: 6265 MHz]",
		},
		{
			name:      "Fallback without iw info using freq only",
			iwChannel: "",
			freqStr:   "5785",
			expected:  "5GHz / CH 157 (5785 MHz)",
		},
		{
			name:      "Fallback 2.4GHz using freq only",
			iwChannel: "",
			freqStr:   "2412",
			expected:  "2.4GHz / CH 1 (2412 MHz)",
		},
		{
			name:      "Empty channel and freq",
			iwChannel: "",
			freqStr:   "",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatChannelInfo(tt.iwChannel, tt.freqStr, false)
			if result != tt.expected {
				t.Errorf("formatChannelInfo(%q, %q) = %q, want %q", tt.iwChannel, tt.freqStr, result, tt.expected)
			}
		})
	}
}

func TestCalcChannelFromFreq(t *testing.T) {
	tests := []struct {
		freq int
		want int
	}{
		{2412, 1},
		{2437, 6},
		{2472, 13},
		{2484, 14},
		{5180, 36},
		{5785, 157},
		{5955, 1},
		{6135, 37},
		{900, 0},
	}

	for _, tt := range tests {
		got := calcChannelFromFreq(tt.freq)
		if got != tt.want {
			t.Errorf("calcChannelFromFreq(%d) = %d, want %d", tt.freq, got, tt.want)
		}
	}
}

func TestFormatBitrateDisplay(t *testing.T) {
	tests := []struct {
		name     string
		tx       string
		rx       string
		expected string
	}{
		{
			name:     "Both TX and RX identical with MCS",
			tx:       "866.7 MBit/s VHT-MCS 9 80MHz short GI VHT-NSS 2",
			rx:       "866.7 MBit/s VHT-MCS 9 80MHz short GI VHT-NSS 2",
			expected: "866.7 MBit/s (VHT-MCS 9)",
		},
		{
			name:     "Different TX and RX",
			tx:       "866.7 MBit/s VHT-MCS 9",
			rx:       "400.0 MBit/s VHT-MCS 4",
			expected: "TX: 866.7 MBit/s (VHT-MCS 9) / RX: 400.0 MBit/s (VHT-MCS 4)",
		},
		{
			name:     "Only TX from LINKSPEED",
			tx:       "866 Mbps",
			rx:       "",
			expected: "866 Mbps",
		},
		{
			name:     "Empty",
			tx:       "",
			rx:       "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBitrateDisplay(tt.tx, tt.rx)
			if got != tt.expected {
				t.Errorf("formatBitrateDisplay(%q, %q) = %q, want %q", tt.tx, tt.rx, got, tt.expected)
			}
		})
	}
}

func TestFormatTxPower(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"txpower 0.00 dBm", "Auto (動態控制)"},
		{"0.00 dBm", "Auto (動態控制)"},
		{"txpower 63.00 dBm", "Auto (上限 63 dBm)"},
		{"txpower 20.00 dBm", "20.00 dBm"},
		{"txpower 42949572.96 dBm", "未知 (Driver 未提供)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := FormatTxPower(tt.input)
			if got != tt.expected {
				t.Errorf("FormatTxPower(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestConfigHasNetwork(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Empty or global only
	f1 := filepath.Join(tmpDir, "wpa1.conf")
	os.WriteFile(f1, []byte("ctrl_interface=/var/run/wpa_supplicant\nupdate_config=1\n"), 0644)
	c1 := &Connector{configPath: f1}
	if c1.configHasNetwork() {
		t.Errorf("Expected false for config without network blocks")
	}

	// 2. Only DummyAP_For_Test
	f2 := filepath.Join(tmpDir, "wpa2.conf")
	os.WriteFile(f2, []byte("network={\n\tssid=\"DummyAP_For_Test\"\n\tkey_mgmt=NONE\n}\n"), 0644)
	c2 := &Connector{configPath: f2}
	if c2.configHasNetwork() {
		t.Errorf("Expected false for config with only DummyAP_For_Test")
	}

	// 3. Real network
	f3 := filepath.Join(tmpDir, "wpa3.conf")
	os.WriteFile(f3, []byte("network={\n\tssid=\"MyHomeAP\"\n\tpsk=\"password\"\n}\n"), 0644)
	c3 := &Connector{configPath: f3}
	if !c3.configHasNetwork() {
		t.Errorf("Expected true for config with real network")
	}

	// 4. Coexistence of DummyAP and Real network
	f4 := filepath.Join(tmpDir, "wpa4.conf")
	os.WriteFile(f4, []byte("network={\n\tssid=\"DummyAP_For_Test\"\n\tkey_mgmt=NONE\n}\nnetwork={\n\tssid=\"MyHomeAP\"\n\tpsk=\"password\"\n}\n"), 0644)
	c4 := &Connector{configPath: f4}
	if !c4.configHasNetwork() {
		t.Errorf("Expected true when real network coexists with DummyAP")
	}

	// 5. Spaced format: ssid = "SpacedAP"
	f5 := filepath.Join(tmpDir, "wpa5.conf")
	os.WriteFile(f5, []byte("network={\n\tssid = \"SpacedAP\"\n\tpsk=\"password\"\n}\n"), 0644)
	c5 := &Connector{configPath: f5}
	if !c5.configHasNetwork() {
		t.Errorf("Expected true for config with spaced ssid")
	}

	// 6. Commented out network: # ssid="CommentedAP"
	f6 := filepath.Join(tmpDir, "wpa6.conf")
	os.WriteFile(f6, []byte("network={\n\t# ssid=\"CommentedAP\"\n\tkey_mgmt=NONE\n}\n"), 0644)
	c6 := &Connector{configPath: f6}
	if c6.configHasNetwork() {
		t.Errorf("Expected false for config with only commented ssid")
	}

	// 7. Empty ssid: ssid=""
	f7 := filepath.Join(tmpDir, "wpa7.conf")
	os.WriteFile(f7, []byte("network={\n\tssid=\"\"\n\tkey_mgmt=NONE\n}\n"), 0644)
	c7 := &Connector{configPath: f7}
	if c7.configHasNetwork() {
		t.Errorf("Expected false for config with empty ssid")
	}
}

func TestParseMorseChannelOutput(t *testing.T) {
	sample := `Full Channel Information
	Operating Frequency: 924000 kHz
	Operating BW: 8 MHz
	Primary BW: 2 MHz
	Primary Channel Index: 3`

	info := parseMorseChannelOutput(sample)
	if info == nil {
		t.Fatalf("Expected non-nil MorseChannelInfo")
	}
	if info.OperatingFreqKHz != 924000 {
		t.Errorf("Expected OperatingFreqKHz 924000, got %d", info.OperatingFreqKHz)
	}
	if info.OperatingBWMHz != 8 {
		t.Errorf("Expected OperatingBWMHz 8, got %d", info.OperatingBWMHz)
	}
	if info.PrimaryBWMHz != 2 {
		t.Errorf("Expected PrimaryBWMHz 2, got %d", info.PrimaryBWMHz)
	}
	if info.PrimaryIndex != 3 {
		t.Errorf("Expected PrimaryIndex 3, got %d", info.PrimaryIndex)
	}
}

func TestFormatMorseChannelDisplay(t *testing.T) {
	info := &MorseChannelInfo{
		OperatingFreqKHz: 924000,
		OperatingBWMHz:   8,
		PrimaryBWMHz:     2,
		PrimaryIndex:     3,
	}

	iwChannel := "channel 161 (5805 MHz), width: 160 MHz, center1: 5815 MHz"
	got := formatMorseChannelDisplay(info, iwChannel)
	expected := "Sub-GHz (HaLow) / 924 MHz [8 MHz, Primary: 2 MHz] (Mapped: CH 161)"
	if got != expected {
		t.Errorf("formatMorseChannelDisplay() = %q, want %q", got, expected)
	}

	// Without primary BW difference
	info2 := &MorseChannelInfo{
		OperatingFreqKHz: 920000,
		OperatingBWMHz:   2,
		PrimaryBWMHz:     2,
	}
	got2 := formatMorseChannelDisplay(info2, "channel 36 (5180 MHz)")
	expected2 := "Sub-GHz (HaLow) / 920 MHz [2 MHz] (Mapped: CH 36)"
	if got2 != expected2 {
		t.Errorf("formatMorseChannelDisplay() = %q, want %q", got2, expected2)
	}
}

func TestParseMorseStatsOutput(t *testing.T) {
	sample := `Noise (dBm)                                     : -89
Medium energy (dBm)                             : -86
SNR estimated (dB)                              : 36
MCS max estimated                               : 9
MCS min estimated                               : 9
PHY rate max (kbps)                             : 39000
PHY rate min (kbps)                             : 39000`

	info := parseMorseStatsOutput(sample)
	if info == nil {
		t.Fatalf("Expected non-nil MorseStatsInfo")
	}
	if info.PhyRateMaxKbps != 39000 {
		t.Errorf("Expected PhyRateMaxKbps 39000, got %d", info.PhyRateMaxKbps)
	}
	if info.PhyRateMinKbps != 39000 {
		t.Errorf("Expected PhyRateMinKbps 39000, got %d", info.PhyRateMinKbps)
	}
}

func TestFormatMorseBitrateDisplay(t *testing.T) {
	info := &MorseStatsInfo{
		PhyRateMaxKbps: 39000,
		PhyRateMinKbps: 39000,
	}

	emulated := "650.0 MBit/s (VHT-MCS 7)"
	got := formatMorseBitrateDisplay(info, emulated)
	expected := "39.0 Mbps (S1G PHY) [VHT Emulated: 650.0 MBit/s (VHT-MCS 7)]"
	if got != expected {
		t.Errorf("formatMorseBitrateDisplay() = %q, want %q", got, expected)
	}

	// Without emulated
	gotNoEmul := formatMorseBitrateDisplay(info, "")
	expectedNoEmul := "39.0 Mbps (S1G PHY Rate)"
	if gotNoEmul != expectedNoEmul {
		t.Errorf("formatMorseBitrateDisplay() = %q, want %q", gotNoEmul, expectedNoEmul)
	}
}

