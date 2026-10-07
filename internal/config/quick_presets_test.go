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

	// Check overall presets count: 30 total (9 6G + 14 5G + 6 2.4G + 1 sub-GHz)
	allPresets := getAllPresets()
	if len(allPresets) != 30 {
		t.Fatalf("Expected 30 total presets in getAllPresets(), got %d", len(allPresets))
	}

	p6g := filterPresets(fullCaps, "6G")
	if len(p6g) != 9 {
		t.Errorf("Expected 9 presets for full 6G, got %d", len(p6g))
	}
	if p6g[0].Mode != "be" || p6g[0].Bandwidth != 320 {
		t.Errorf("Expected first preset to be be-320M, got %s-%dM", p6g[0].Mode, p6g[0].Bandwidth)
	}

	p5gFull := filterPresets(fullCaps, "5G")
	if len(p5gFull) != 14 {
		t.Errorf("Expected 14 presets for full 5G, got %d", len(p5gFull))
	}

	p24gFull := filterPresets(fullCaps, "2.4G")
	if len(p24gFull) != 6 {
		t.Errorf("Expected 6 presets for full 2.4G, got %d", len(p24gFull))
	}

	triBandTotal := len(p6g) + len(p5gFull) + len(p24gFull)
	if triBandTotal != 29 {
		t.Fatalf("Expected 29 presets for tri-band (2.4G+5G+6G) module, got %d", triBandTotal)
	}

	// Verify all new presets are present in 5G for fullCaps
	presetMap5G := make(map[string]bool)
	for _, p := range p5gFull {
		presetMap5G[p.Label] = true
	}
	expected5G := []string{
		"WiFi 7 (802.11be) - 40 MHz",
		"WiFi 7 (802.11be) - 20 MHz",
		"WiFi 5 (802.11ac) - 160 MHz",
		"WiFi 5 (802.11ac) - 20 MHz",
	}
	for _, exp := range expected5G {
		if !presetMap5G[exp] {
			t.Errorf("Expected 5G preset %q to be present", exp)
		}
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

func TestNewPresetsGenerateHostapd(t *testing.T) {
	module := &models.WiFiModule{
		ID:          "test_wifi7_module",
		DisplayName: "Test WiFi 7 Module",
		Kernel: models.KernelConfig{
			Interface:     "wlan0",
			DriverBackend: "nl80211",
		},
	}

	// 1. 5G 11ac 160M
	e5gAc160 := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "5G", Mode: "ac", Bandwidth: 160, Channel: 36, SSID: "Test5G", Security: "Open",
		},
	}
	conf5gAc160 := e5gAc160.generateHostapd()
	if !strings.Contains(conf5gAc160, "vht_oper_chwidth=2") || !strings.Contains(conf5gAc160, "vht_oper_centr_freq_seg0_idx=50") {
		t.Errorf("5G ac 160M missing vht params: %s", conf5gAc160)
	}

	// 2. 5G 11ac 20M
	e5gAc20 := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "5G", Mode: "ac", Bandwidth: 20, Channel: 36, SSID: "Test5G", Security: "Open",
		},
	}
	conf5gAc20 := e5gAc20.generateHostapd()
	if !strings.Contains(conf5gAc20, "ht_capab=[HT20]") || strings.Contains(conf5gAc20, "vht_oper_chwidth") {
		t.Errorf("5G ac 20M should have HT20 and no vht_oper_chwidth: %s", conf5gAc20)
	}

	// 3. 5G 11be 40M
	e5gBe40 := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "5G", Mode: "be", Bandwidth: 40, Channel: 36, SSID: "Test5G", Security: "Open",
		},
	}
	conf5gBe40 := e5gBe40.generateHostapd()
	if !strings.Contains(conf5gBe40, "ieee80211be=1") || !strings.Contains(conf5gBe40, "ht_capab=[HT40+]") || strings.Contains(conf5gBe40, "eht_oper_chwidth") {
		t.Errorf("5G be 40M should have 11be and HT40+ without eht_oper_chwidth: %s", conf5gBe40)
	}

	// 4. 5G 11be 20M
	e5gBe20 := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "5G", Mode: "be", Bandwidth: 20, Channel: 36, SSID: "Test5G", Security: "Open",
		},
	}
	conf5gBe20 := e5gBe20.generateHostapd()
	if !strings.Contains(conf5gBe20, "ieee80211be=1") || !strings.Contains(conf5gBe20, "ht_capab=[HT20]") || strings.Contains(conf5gBe20, "eht_oper_chwidth") {
		t.Errorf("5G be 20M should have 11be and HT20 without eht_oper_chwidth: %s", conf5gBe20)
	}

	// 5. 6G 11be 40M
	e6gBe40 := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "6G", Mode: "be", Bandwidth: 40, Channel: 37, SSID: "Test6G", Security: "WPA3",
		},
	}
	conf6gBe40 := e6gBe40.generateHostapd()
	if !strings.Contains(conf6gBe40, "op_class=132") || !strings.Contains(conf6gBe40, "eht_oper_centr_freq_seg0_idx=35") || !strings.Contains(conf6gBe40, "eht_oper_chwidth=0") {
		t.Errorf("6G be 40M should have op_class=132 and center 35: %s", conf6gBe40)
	}

	// 6. 6G 11be 20M
	e6gBe20 := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "6G", Mode: "be", Bandwidth: 20, Channel: 37, SSID: "Test6G", Security: "WPA3",
		},
	}
	conf6gBe20 := e6gBe20.generateHostapd()
	if !strings.Contains(conf6gBe20, "op_class=131") || strings.Contains(conf6gBe20, "eht_oper_chwidth") {
		t.Errorf("6G be 20M should have op_class=131 without eht_oper_chwidth: %s", conf6gBe20)
	}

	// 7. 6G 11be 320M (Upper sub-band PSC 37 -> HE center 47, EHT center 31)
	e6gBe320Upper := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "6G", Mode: "be", Bandwidth: 320, Channel: 37, SSID: "Test6G_320", Security: "WPA3",
		},
	}
	conf6gBe320Upper := e6gBe320Upper.generateHostapd()
	if !strings.Contains(conf6gBe320Upper, "op_class=137") {
		t.Errorf("6G be 320M missing op_class=137: %s", conf6gBe320Upper)
	}
	if !strings.Contains(conf6gBe320Upper, "he_oper_centr_freq_seg0_idx=47") {
		t.Errorf("6G be 320M upper sub-band should have he_oper_centr_freq_seg0_idx=47: %s", conf6gBe320Upper)
	}
	if !strings.Contains(conf6gBe320Upper, "eht_oper_centr_freq_seg0_idx=31") {
		t.Errorf("6G be 320M should have eht_oper_centr_freq_seg0_idx=31: %s", conf6gBe320Upper)
	}

	// 8. 6G 11be 320M (Lower sub-band PSC 5 -> HE center 15, EHT center 31)
	e6gBe320Lower := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "6G", Mode: "be", Bandwidth: 320, Channel: 5, SSID: "Test6G_320_Low", Security: "WPA3",
		},
	}
	conf6gBe320Lower := e6gBe320Lower.generateHostapd()
	if !strings.Contains(conf6gBe320Lower, "he_oper_centr_freq_seg0_idx=15") {
		t.Errorf("6G be 320M lower sub-band should have he_oper_centr_freq_seg0_idx=15: %s", conf6gBe320Lower)
	}
	if !strings.Contains(conf6gBe320Lower, "eht_oper_centr_freq_seg0_idx=31") {
		t.Errorf("6G be 320M lower sub-band should have eht_oper_centr_freq_seg0_idx=31: %s", conf6gBe320Lower)
	}
	if !strings.Contains(conf6gBe320Lower, "channel=5\n") {
		t.Errorf("6G be 320M lower sub-band should have channel=5: %s", conf6gBe320Lower)
	}

	// 9. 5G DFS generation check (ch52, 80MHz)
	e5gDfs := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "5G", Mode: "ax", Bandwidth: 80, Channel: 52, CountryCode: "US", SSID: "TestDFS", Security: "WPA2",
		},
	}
	conf5gDfs := e5gDfs.generateHostapd()
	if !strings.Contains(conf5gDfs, "ieee80211h=1") {
		t.Errorf("5G DFS ch52 missing ieee80211h=1: %s", conf5gDfs)
	}
	if strings.Contains(conf5gDfs, "country_ie_env") {
		t.Errorf("5G DFS should not contain non-standard country_ie_env: %s", conf5gDfs)
	}
	if strings.Count(conf5gDfs, "ieee80211d=1") != 1 {
		t.Errorf("5G DFS ieee80211d=1 should appear exactly once, got %d in: %s", strings.Count(conf5gDfs, "ieee80211d=1"), conf5gDfs)
	}

	// 10. 5G UNII-3 ch149 80MHz should NOT have ieee80211h=1 (not DFS)
	e5gUnii3 := &APConfigEditor{
		module: module,
		params: APEditorParams{
			Band: "5G", Mode: "ac", Bandwidth: 80, Channel: 149, CountryCode: "US", SSID: "TestUNII3", Security: "Open",
		},
	}
	conf5gUnii3 := e5gUnii3.generateHostapd()
	if strings.Contains(conf5gUnii3, "ieee80211h=1") {
		t.Errorf("5G UNII-3 ch149 80MHz should NOT have ieee80211h=1 (not DFS): %s", conf5gUnii3)
	}

	// 10b. Realtek (rtw_country_code) DFS: hostapd rejects ieee80211h=1 without ieee80211d=1 + country_code
	rtwModule := &models.WiFiModule{
		ID:          "test_rtw_module",
		DisplayName: "Test Realtek Module",
		Kernel: models.KernelConfig{
			Interface:     "wlan0",
			DriverBackend: "nl80211",
			InsmodArgs:    "rtw_country_code=CN",
		},
	}
	eRtwDfs := &APConfigEditor{
		module: rtwModule,
		params: APEditorParams{
			Band: "5G", Mode: "ac", Bandwidth: 80, Channel: 52, CountryCode: "CN", SSID: "TestRtwDFS", Security: "Open",
		},
	}
	confRtwDfs := eRtwDfs.generateHostapd()
	for _, tok := range []string{"ieee80211h=1\n", "ieee80211d=1\n", "country_code=CN\n"} {
		if strings.Count(confRtwDfs, tok) != 1 {
			t.Errorf("Realtek DFS should contain exactly one %q, got:\n%s", tok, confRtwDfs)
		}
	}
	eRtwNonDfs := &APConfigEditor{
		module: rtwModule,
		params: APEditorParams{
			Band: "5G", Mode: "ac", Bandwidth: 80, Channel: 36, CountryCode: "CN", SSID: "TestRtwNonDFS", Security: "Open",
		},
	}
	if confRtwNonDfs := eRtwNonDfs.generateHostapd(); strings.Contains(confRtwNonDfs, "country_code=") {
		t.Errorf("Realtek non-DFS should leave regulatory to rtw_country_code, got:\n%s", confRtwNonDfs)
	}

	// 11. Sub-GHz (FGH200M) should preserve op_class from FixedParams (e.g. op_class=70)
	fghModule := &models.WiFiModule{
		ID:          "quectel_fgh200m",
		DisplayName: "Quectel FGH200M",
		Kernel: models.KernelConfig{
			Interface:     "wlan0",
			DriverBackend: "nl80211",
		},
		APConfig: models.APConfig{
			FixedParams: []models.Param{
				{Key: "op_class", Value: "70"},
				{Key: "s1g_prim_chwidth", Value: "1"},
			},
		},
	}
	eFgh := &APConfigEditor{
		module: fghModule,
		params: APEditorParams{
			Band: "sub-GHz", Mode: "ah", Bandwidth: 8, Channel: 40, CountryCode: "AU", SSID: "Quectel-halow", Security: "WPA3",
		},
	}
	confFgh := eFgh.generateHostapd()
	if !strings.Contains(confFgh, "op_class=70\n") {
		t.Errorf("Sub-GHz FGH200M missing op_class=70 from FixedParams: %s", confFgh)
	}

	// 12. Non-sub-GHz (e.g. 5G/2.4G) should NOT leak op_class from FixedParams
	moduleWithOpClass := &models.WiFiModule{
		ID:          "test_wifi7_module",
		DisplayName: "Test WiFi 7 Module",
		Kernel: models.KernelConfig{
			Interface:     "wlan0",
			DriverBackend: "nl80211",
		},
		APConfig: models.APConfig{
			FixedParams: []models.Param{
				{Key: "op_class", Value: "137"},
			},
		},
	}
	e5gLeakCheck := &APConfigEditor{
		module: moduleWithOpClass,
		params: APEditorParams{
			Band: "5G", Mode: "ax", Bandwidth: 80, Channel: 36, CountryCode: "US", SSID: "Test5G_NoOpClass", Security: "Open",
		},
	}
	conf5gLeak := e5gLeakCheck.generateHostapd()
	if strings.Contains(conf5gLeak, "op_class=") {
		t.Errorf("5G hostapd config should NOT contain op_class, got:\n%s", conf5gLeak)
	}
}


