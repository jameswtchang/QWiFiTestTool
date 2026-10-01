package models

import (
	"testing"
)

func TestChannelToFreq(t *testing.T) {
	tests := []struct {
		band     string
		ch       int
		expected int
	}{
		{"2.4G", 1, 2412},
		{"2.4G", 6, 2437},
		{"2.4G", 11, 2462},
		{"2.4G", 14, 2484},
		{"5G", 36, 5180},
		{"5G", 52, 5260},
		{"5G", 100, 5500},
		{"5G", 149, 5745},
		{"5G", 165, 5825},
		{"6G", 1, 5955},
		{"6G", 2, 5935},
		{"6G", 5, 5975},
		{"6G", 33, 6115},
		{"6G", 233, 7115},
	}

	for _, tt := range tests {
		got := ChannelToFreq(tt.band, tt.ch)
		if got != tt.expected {
			t.Errorf("ChannelToFreq(%s, %d) = %d, expected %d", tt.band, tt.ch, got, tt.expected)
		}
	}
}

func TestIsDFSChannel(t *testing.T) {
	if IsDFSChannel(36) {
		t.Errorf("Channel 36 should not be DFS")
	}
	if !IsDFSChannel(52) {
		t.Errorf("Channel 52 should be DFS")
	}
	if !IsDFSChannel(144) {
		t.Errorf("Channel 144 should be DFS")
	}
	if IsDFSChannel(149) {
		t.Errorf("Channel 149 should not be DFS")
	}
}

func TestParseChannelString(t *testing.T) {
	tests := []struct {
		input    string
		expected []int
	}{
		{"", nil},
		{"-", nil},
		{"  ", nil},
		{"1", []int{1}},
		{"1-5", []int{1, 2, 3, 4, 5}},
		{"36, 40, 44, 48", []int{36, 40, 44, 48}},
		{"1-3, 6, 11-13", []int{1, 2, 3, 6, 11, 12, 13}},
	}

	for _, tt := range tests {
		got := ParseChannelString(tt.input)
		if len(got) != len(tt.expected) {
			t.Fatalf("ParseChannelString(%q) got length %d, expected %d", tt.input, len(got), len(tt.expected))
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("ParseChannelString(%q)[%d] = %d, expected %d", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestLookupChannels(t *testing.T) {
	db, err := LoadChannelLookup()
	if err != nil {
		t.Fatalf("LoadChannelLookup failed: %v", err)
	}

	// 1. ListCountryCodes & FindCountry
	codes := db.ListCountryCodes()
	if len(codes) == 0 {
		t.Fatalf("ListCountryCodes should not be empty")
	}

	usUpper := db.FindCountry("US")
	usLower := db.FindCountry("us")
	if usUpper == nil || usLower == nil || usUpper != usLower {
		t.Fatalf("FindCountry should be case-insensitive, got %v and %v", usUpper, usLower)
	}

	apTypes := usUpper.GetAPTypes()
	if len(apTypes) == 0 {
		t.Fatalf("US should have at least one AP type")
	}

	// 2. Comprehensive Matrix tests
	matrixTests := []struct {
		country   string
		apType    string
		band      string
		bandwidth int
		minChans  int
	}{
		// US
		{"US", "所有类型AP", "2.4G", 20, 11}, // 1-11
		{"US", "所有类型AP", "2.4G", 40, 7},  // 40M
		{"US", "所有类型AP", "5G", 20, 20},
		{"US", "所有类型AP", "5G", 40, 10},
		{"US", "所有类型AP", "5G", 80, 5},
		{"US", "所有类型AP", "5G", 160, 2},
		{"US", "所有类型AP", "6G", 20, 50},
		{"US", "所有类型AP", "6G", 80, 10},
		{"US", "所有类型AP", "6G", 160, 5},
		{"US", "所有类型AP", "6G", 320, 2}, // 320MHz

		// CN
		{"CN", "所有类型AP", "2.4G", 20, 13}, // 1-13
		{"CN", "所有类型AP", "5G", 20, 5},    // 149-165
		{"CN", "所有类型AP", "5G", 80, 1},    // 149 only (outdoor safe)
		{"CN", "室内型AP", "5G", 80, 3},      // 36, 52, 149

		// TW
		{"TW", "所有类型AP", "2.4G", 20, 11},
		{"TW", "所有类型AP", "5G", 20, 15},
		{"TW", "所有类型AP", "6G", 20, 10},

		// JP
		{"JP", "所有类型AP", "2.4G", 20, 13}, // 1-13
		{"JP", "所有类型AP", "5G", 20, 12},   // W56 (100-144) only for outdoor/all
		{"JP", "室内型AP", "5G", 20, 19},     // W52 + W53 + W56
	}

	for _, tt := range matrixTests {
		chans := db.LookupChannels(tt.country, tt.apType, tt.band, tt.bandwidth)
		if len(chans) < tt.minChans {
			t.Errorf("LookupChannels(%s, %s, %s, %dM) returned %d channels (%v), expected at least %d",
				tt.country, tt.apType, tt.band, tt.bandwidth, len(chans), chans, tt.minChans)
		}
	}
}

func TestLookupChannels_Fallbacks(t *testing.T) {
	db, err := LoadChannelLookup()
	if err != nil {
		t.Fatalf("LoadChannelLookup failed: %v", err)
	}

	// Non-existent country returns nil
	if chans := db.LookupChannels("NONEXISTENT", "所有类型AP", "2.4G", 20); chans != nil {
		t.Errorf("Expected nil for invalid country, got %v", chans)
	}

	// Fallback to "所有类型AP" when nonexistent AP type requested
	chans := db.LookupChannels("US", "NonExistentAPType", "2.4G", 20)
	if len(chans) == 0 {
		t.Errorf("Expected fallback channels for US when invalid AP type requested")
	}
}

func TestChannelLookup_Demonstrate(t *testing.T) {
	db, err := LoadChannelLookup()
	if err != nil {
		t.Fatalf("LoadChannelLookup failed: %v", err)
	}

	queries := []struct {
		country string
		apType  string
		band    string
		bw      int
	}{
		{"TW", "室内型AP", "6G", 320},
		{"TW", "室内型AP", "6G", 160},
		{"TW", "所有类型AP", "5G", 80},
		{"US", "室内型AP", "6G", 320},
		{"US", "所有类型AP", "5G", 160},
		{"CN", "室内型AP", "5G", 80},
		{"CN", "所有类型AP", "2.4G", 20},
		{"JP", "室内型AP", "5G", 80},
	}

	for _, q := range queries {
		chans := db.LookupChannels(q.country, q.apType, q.band, q.bw)
		t.Logf("[%s | %s] %s %dMHz -> Channels (%d): %v", q.country, q.apType, q.band, q.bw, len(chans), chans)
	}
}

func TestKernelPreloadArgsList(t *testing.T) {
	k := &KernelConfig{
		PreloadKo:         "dot11ah.ko",
		PreloadInsmodArgs: "channelization_scheme=1",
	}
	koList := k.GetPreloadKoList()
	if len(koList) != 1 || koList[0] != "dot11ah.ko" {
		t.Fatalf("unexpected preload ko list: %v", koList)
	}
	argsList := k.GetPreloadArgsList()
	if len(argsList) != 1 || argsList[0] != "channelization_scheme=1" {
		t.Fatalf("unexpected preload args list: %v", argsList)
	}

	// Test multi-preload
	k2 := &KernelConfig{
		PreloadKo:         "mod1.ko, mod2.ko",
		PreloadInsmodArgs: "arg1=val1, arg2=val2",
	}
	argsList2 := k2.GetPreloadArgsList()
	if len(argsList2) != 2 || argsList2[0] != "arg1=val1" || argsList2[1] != "arg2=val2" {
		t.Fatalf("unexpected multi preload args list: %v", argsList2)
	}
}

func TestQuectelFGH200M_DefaultModuleConfig(t *testing.T) {
	cfg, err := LoadModules()
	if err != nil {
		t.Fatalf("failed to load default modules: %v", err)
	}
	var fgh *WiFiModule
	for i := range cfg.Modules {
		if cfg.Modules[i].ID == "quectel_fgh200m" {
			fgh = &cfg.Modules[i]
			break
		}
	}
	if fgh == nil {
		t.Fatalf("quectel_fgh200m module not found")
	}
	if fgh.Kernel.PreloadInsmodArgs != "channelization_scheme=1" {
		t.Errorf("expected PreloadInsmodArgs to be 'channelization_scheme=1', got '%s'", fgh.Kernel.PreloadInsmodArgs)
	}
	if fgh.Kernel.InsmodArgs != "country=AU bcf=bcf_default.bin" {
		t.Errorf("expected InsmodArgs to be 'country=AU bcf=bcf_default.bin', got '%s'", fgh.Kernel.InsmodArgs)
	}
}

