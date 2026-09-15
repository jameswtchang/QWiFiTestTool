package config

import (
	"strings"
	"testing"

	"qwifitesttool/internal/models"
)

func TestFilterPresets(t *testing.T) {
	// Case 1: Full capabilities (WiFi 7, 320M, 6G)
	fullCaps := &models.APCapabilities{
		Bands:        "2.4G,5G,6G",
		Modes:        "b,g,n,ac,ax,be",
		MaxBandwidth: 320,
		SupportWPA3:  true,
	}

	p6g := filterPresets(fullCaps, "6G")
	if len(p6g) != 7 {
		t.Errorf("Expected 7 presets for full 6G, got %d", len(p6g))
	}
	if p6g[0].Mode != "be" || p6g[0].Bandwidth != 320 {
		t.Errorf("Expected first preset to be be-320M, got %s-%dM", p6g[0].Mode, p6g[0].Bandwidth)
	}

	// Case 2: WiFi 6 module (max 80M, no 6G, no WiFi 7)
	axCaps := &models.APCapabilities{
		Bands:        "2.4G,5G",
		Modes:        "b,g,n,ac,ax",
		MaxBandwidth: 80,
	}

	p5g := filterPresets(axCaps, "5G")
	for _, p := range p5g {
		if p.Mode == "be" {
			t.Errorf("Should not include WiFi 7 for axCaps")
		}
		if p.Bandwidth > 80 {
			t.Errorf("Should not include bandwidth > 80M, got %d", p.Bandwidth)
		}
	}

	// 6G should be empty for axCaps
	p6gEmpty := filterPresets(axCaps, "6G")
	if len(p6gEmpty) != 0 {
		t.Errorf("Expected 0 presets for 6G on axCaps, got %d", len(p6gEmpty))
	}
}

func TestGetAvailableBands(t *testing.T) {
	fullCaps := &models.APCapabilities{
		Bands:        "2.4G,5G,6G",
		Modes:        "b,g,n,ac,ax,be",
		MaxBandwidth: 320,
	}

	bands := getAvailableBands(fullCaps)
	if len(bands) != 3 {
		t.Fatalf("Expected 3 bands, got %d", len(bands))
	}
	if bands[0].Band != "2.4G" || bands[0].MaxLabel != "be-40M" {
		t.Errorf("2.4G max label unexpected: %s", bands[0].MaxLabel)
	}
	if bands[1].Band != "5G" || bands[1].MaxLabel != "be-160M" {
		t.Errorf("5G max label unexpected: %s", bands[1].MaxLabel)
	}
	if bands[2].Band != "6G" || bands[2].MaxLabel != "be-320M" {
		t.Errorf("6G max label unexpected: %s", bands[2].MaxLabel)
	}
}

func TestCalcPresetCenterChannel(t *testing.T) {
	// 6G
	if c := calcPresetCenterChannel("6G", 320, 37); c != 31 {
		t.Errorf("Expected 6G 320M ch37 center to be 31, got %d", c)
	}
	if c := calcPresetCenterChannel("6G", 160, 37); c != 47 {
		t.Errorf("Expected 6G 160M ch37 center to be 47, got %d", c)
	}
	if c := calcPresetCenterChannel("6G", 80, 37); c != 39 {
		t.Errorf("Expected 6G 80M ch37 center to be 39, got %d", c)
	}
	if c := calcPresetCenterChannel("6G", 40, 37); c != 35 {
		t.Errorf("Expected 6G 40M ch37 center to be 35, got %d", c)
	}

	// 5G
	if c := calcPresetCenterChannel("5G", 160, 36); c != 50 {
		t.Errorf("Expected 5G 160M ch36 center to be 50, got %d", c)
	}
	if c := calcPresetCenterChannel("5G", 160, 149); c != 163 {
		t.Errorf("Expected 5G 160M ch149 center to be 163, got %d", c)
	}
	if c := calcPresetCenterChannel("5G", 80, 36); c != 42 {
		t.Errorf("Expected 5G 80M ch36 center to be 42, got %d", c)
	}
}

func TestCalcPrimaryChannel6G(t *testing.T) {
	// 80MHz: Center 39 -> Primary PSC 37, Center 7 -> Primary PSC 5
	if ch := CalcPrimaryChannel6G(39, 80); ch != 37 {
		t.Errorf("Expected 6G 80M center 39 primary to be 37, got %d", ch)
	}
	if ch := CalcPrimaryChannel6G(7, 80); ch != 5 {
		t.Errorf("Expected 6G 80M center 7 primary to be 5, got %d", ch)
	}

	// 160MHz: Center 47 -> Primary PSC 37, Center 15 -> Primary PSC 5
	if ch := CalcPrimaryChannel6G(47, 160); ch != 37 {
		t.Errorf("Expected 6G 160M center 47 primary to be 37, got %d", ch)
	}
	if ch := CalcPrimaryChannel6G(15, 160); ch != 5 {
		t.Errorf("Expected 6G 160M center 15 primary to be 5, got %d", ch)
	}

	// 320MHz: Center 31 -> Primary PSC 37
	if ch := CalcPrimaryChannel6G(31, 320); ch != 37 {
		t.Errorf("Expected 6G 320M center 31 primary to be 37, got %d", ch)
	}

	// 40MHz: Center 35 -> Primary PSC 37, Center 3 -> Primary PSC 5
	if ch := CalcPrimaryChannel6G(35, 40); ch != 37 {
		t.Errorf("Expected 6G 40M center 35 primary to be 37, got %d", ch)
	}
	if ch := CalcPrimaryChannel6G(3, 40); ch != 5 {
		t.Errorf("Expected 6G 40M center 3 primary to be 5, got %d", ch)
	}

	// 20MHz: should remain unchanged
	if ch := CalcPrimaryChannel6G(37, 20); ch != 37 {
		t.Errorf("Expected 6G 20M ch 37 primary to be 37, got %d", ch)
	}
}

func TestGenerateHostapdFiltering(t *testing.T) {
	module := &models.WiFiModule{
		ID:          "quectel_fme170q_865",
		DisplayName: "Quectel FME170Q-865",
		APConfig: models.APConfig{
			FixedParams: []models.Param{
				{Key: "beacon_int", Value: "100"},
				{Key: "acs_exclude_6ghz_non_psc", Value: "1"},
				{Key: "he_bss_color", Value: "15"},
				{Key: "he_su_beamformer", Value: "1"},
				{Key: "eht_oper_chwidth", Value: "3"},
				{Key: "ieee80211w", Value: "2"},
				{Key: "wpa_pairwise", Value: "CCMP"},
			},
		},
	}

	editor := &APConfigEditor{
		module: module,
		params: APEditorParams{
			CountryCode: "TW",
			Band:        "5G",
			Mode:        "ac",
			Bandwidth:   80,
			Channel:     36,
			SSID:        "Test_AP",
			Security:    "WPA3",
			Password:    "12345678",
		},
	}

	conf := editor.generateHostapd()

	// Should contain 5G AC settings
	if !strings.Contains(conf, "hw_mode=a") {
		t.Errorf("Expected hw_mode=a")
	}
	if !strings.Contains(conf, "channel=36") {
		t.Errorf("Expected channel=36")
	}
	if !strings.Contains(conf, "ieee80211ac=1") {
		t.Errorf("Expected ieee80211ac=1")
	}

	// Should NOT contain HE, EHT, or 6G ACS params
	if strings.Contains(conf, "he_bss_color") {
		t.Errorf("5G AC config should not contain he_bss_color")
	}
	if strings.Contains(conf, "he_su_beamformer") {
		t.Errorf("5G AC config should not contain he_su_beamformer")
	}
	if strings.Contains(conf, "eht_oper_chwidth") {
		t.Errorf("5G AC config should not contain eht_oper_chwidth")
	}
	if strings.Contains(conf, "acs_exclude_6ghz_non_psc") {
		t.Errorf("5G config should not contain acs_exclude_6ghz_non_psc")
	}

	// Should contain ieee80211w=2 exactly once (under Security)
	count := strings.Count(conf, "ieee80211w=2")
	if count != 1 {
		t.Errorf("Expected ieee80211w=2 to appear exactly once, got %d", count)
	}

	// Should not have duplicate wpa_pairwise when rsn_pairwise is present
	if strings.Contains(conf, "wpa_pairwise=CCMP") {
		t.Errorf("Should not contain wpa_pairwise when rsn_pairwise is present")
	}

	// Test WPA2 mode skips sae_pwe and ieee80211w from FixedParams
	editor.params.Security = "WPA2"
	editor.module.APConfig.FixedParams = append(editor.module.APConfig.FixedParams, models.Param{Key: "sae_pwe", Value: "2"})
	confWPA2 := editor.generateHostapd()
	if strings.Contains(confWPA2, "sae_pwe") {
		t.Errorf("WPA2 config should not contain sae_pwe")
	}
	if strings.Contains(confWPA2, "ieee80211w") {
		t.Errorf("WPA2 config should not contain mandatory ieee80211w")
	}
}

func TestOpClassBandwidthParsing(t *testing.T) {
	editor := &APConfigEditor{}
	cfg6G40M := map[string]string{
		"hw_mode":  "a",
		"channel":  "37",
		"op_class": "132",
	}
	editor.applyProfileParams(cfg6G40M)
	if editor.params.Band != "6G" {
		t.Errorf("Expected Band=6G, got %s", editor.params.Band)
	}
	if editor.params.Bandwidth != 40 {
		t.Errorf("Expected Bandwidth=40 for op_class 132, got %d", editor.params.Bandwidth)
	}
}


