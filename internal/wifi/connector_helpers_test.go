package wifi

import (
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
			result := formatChannelInfo(tt.iwChannel, tt.freqStr)
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
