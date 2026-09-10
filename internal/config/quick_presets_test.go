package config

import (
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

