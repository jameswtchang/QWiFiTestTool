package wifi

import (
	"strings"
	"testing"
)

func TestDecodeWpaString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"PlainAP", "PlainAP"},
		{`\xe4\xb8\xad\xe6\x96\x87AP`, "中文AP"}, // UTF-8 escaped by wpa_cli
		{`My\"Home\"`, `My"Home"`},             // escaped quotes
		{`back\\slash`, `back\slash`},          // escaped backslash
		{`tab\there`, "tab\there"},             // \t
		{`\x0`, "\x00"},                        // single hex digit
		{`\101BC`, "ABC"},                      // octal
		{`Caf\xc3\xa9 2.4G`, "Café 2.4G"},      // mixed with spaces
		{`trailing\`, `trailing\`},             // dangling backslash kept
	}
	for _, tt := range tests {
		if got := decodeWpaString(tt.in); got != tt.want {
			t.Errorf("decodeWpaString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatSSIDValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"MyAP", `"MyAP"`},
		{"My AP 5G", `"My AP 5G"`},
		{"中文", "e4b8ade69687"},        // UTF-8 -> hex, unquoted
		{`Say"Hi"`, "53617922486922"}, // quote -> hex
	}
	for _, tt := range tests {
		if got := formatSSIDValue(tt.in); got != tt.want {
			t.Errorf("formatSSIDValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatPSKValue(t *testing.T) {
	hexPSK := strings.Repeat("ab", 32) // 64 hex digits
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"12345678", `"12345678"`, false},
		{strings.Repeat("a", 63), `"` + strings.Repeat("a", 63) + `"`, false},
		{hexPSK, hexPSK, false},             // raw PSK must be unquoted
		{"1234567", "", true},               // too short
		{strings.Repeat("z", 64), "", true}, // 64 chars but not hex
		{"pass\nword", "", true},            // line break
		{`quo"te12`, `"quo"te12"`, false},   // wpa_supplicant takes the last quote
	}
	for _, tt := range tests {
		got, err := formatPSKValue(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("formatPSKValue(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("formatPSKValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildNetworkBlock_Variants(t *testing.T) {
	// Chinese SSID + WPA2: hex SSID, quoted passphrase
	block, err := buildNetworkBlock("中文AP", "12345678", "[WPA2-PSK-CCMP][ESS]", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(block, "ssid=e4b8ade69687"+"4150\n") {
		t.Errorf("expected hex SSID, got:\n%s", block)
	}
	if !strings.Contains(block, `psk="12345678"`) {
		t.Errorf("expected quoted psk, got:\n%s", block)
	}

	// SAE: sae_password + PMF required
	block, err = buildNetworkBlock("WPA3AP", "short", "[RSN-SAE-CCMP][ESS]", nil)
	if err != nil {
		t.Fatalf("SAE allows short passwords: %v", err)
	}
	if !strings.Contains(block, `sae_password="short"`) || !strings.Contains(block, "ieee80211w=2") {
		t.Errorf("expected SAE block, got:\n%s", block)
	}

	// Invalid WPA2 passphrase is rejected
	if _, err := buildNetworkBlock("AP", "123", "[WPA2-PSK-CCMP][ESS]", nil); err == nil {
		t.Error("expected error for 3-char WPA2 passphrase")
	}

	// Policies are emitted in stable (sorted) order
	policies := map[string]string{"freq_list": "5180", "disable_he": "1", "disable_eht": "1"}
	block, _ = buildNetworkBlock("AP", "", "[ESS]", policies)
	iEht := strings.Index(block, "disable_eht")
	iHe := strings.Index(block, "disable_he=")
	iFreq := strings.Index(block, "freq_list")
	if !(iEht < iHe && iHe < iFreq) {
		t.Errorf("expected sorted policies, got:\n%s", block)
	}
}

func TestParseScanResults_TabSeparated(t *testing.T) {
	output := "bssid / frequency / signal level / flags / ssid\n" +
		"aa:bb:cc:dd:ee:01\t2437\t-40\t[WPA2-PSK-CCMP][ESS]\tMy Home AP\n" +
		"aa:bb:cc:dd:ee:02\t5180\t-55\t[RSN-SAE-CCMP][ESS]\t\\xe4\\xb8\\xad\\xe6\\x96\\x87\n" +
		"aa:bb:cc:dd:ee:03\t5200\t-60\t[ESS]\t2437\n" + // SSID equal to an earlier field
		"aa:bb:cc:dd:ee:04\t5220\t-70\t[ESS]\t\n" // hidden SSID

	results := (&Scanner{}).parseScanResults(output)
	if len(results) != 3 {
		t.Fatalf("expected 3 results (hidden skipped), got %d: %+v", len(results), results)
	}
	if results[0].SSID != "My Home AP" || results[0].Flags != "[WPA2-PSK-CCMP][ESS]" {
		t.Errorf("result 0 = %+v", results[0])
	}
	if results[1].SSID != "中文" {
		t.Errorf("expected decoded Chinese SSID, got %q", results[1].SSID)
	}
	if results[2].SSID != "2437" || results[2].Frequency != "5200" {
		t.Errorf("SSID equal to a frequency must not be mis-sliced, got %+v", results[2])
	}
}

// Non-tab (space padded) output must still parse via the legacy fallback
func TestParseScanResults_SpaceFallback(t *testing.T) {
	output := "bssid / frequency / signal level / flags / ssid\n" +
		"aa:bb:cc:dd:ee:01  2437  -40  [WPA2-PSK-CCMP][ESS]  My Home AP\n" +
		"aa:bb:cc:dd:ee:02\t5180\t-55\t[ESS]\tTabAP\n"
	results := (&Scanner{}).parseScanResults(output)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}
	if results[0].SSID != "My Home AP" || results[0].Frequency != "2437" || results[0].Flags != "[WPA2-PSK-CCMP][ESS]" {
		t.Errorf("space-padded line parsed wrong: %+v", results[0])
	}
	if results[1].SSID != "TabAP" {
		t.Errorf("tab line parsed wrong: %+v", results[1])
	}
}

func TestNetmaskToPrefix(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"255.255.255.0", 24, false},
		{"255.255.0.0", 16, false},
		{"255.255.255.128", 25, false},
		{"255.255.252.0", 22, false},
		{" 255.255.255.0 ", 24, false},
		{"255.0.255.0", 0, true}, // non-contiguous
		{"not-a-mask", 0, true},
		{"::1", 0, true}, // IPv6
	}
	for _, tt := range tests {
		got, err := netmaskToPrefix(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("netmaskToPrefix(%q) = %d, %v; want %d, err=%v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}
