package config

import (
	"strings"
	"testing"
)

func TestParseConfigAndRebuild(t *testing.T) {
	input := `# wpa_supplicant config
ctrl_interface=/var/run/wpa_supplicant
update_config=1

network={
	ssid="MyHome"
	psk="12345678"
	priority=1
}

network={
	ssid="Guest"
	key_mgmt=NONE
}
`

	cfg := parseConfig(input)

	// Verify global params
	if cfg.GlobalParams["ctrl_interface"] != "/var/run/wpa_supplicant" {
		t.Errorf("Unexpected ctrl_interface: %s", cfg.GlobalParams["ctrl_interface"])
	}
	if cfg.GlobalParams["update_config"] != "1" {
		t.Errorf("Unexpected update_config: %s", cfg.GlobalParams["update_config"])
	}

	// Verify networks
	if len(cfg.Networks) != 2 {
		t.Fatalf("Expected 2 networks, got %d", len(cfg.Networks))
	}
	if cfg.Networks[0].Params["ssid"] != "\"MyHome\"" {
		t.Errorf("Unexpected network 0 ssid: %s", cfg.Networks[0].Params["ssid"])
	}
	if cfg.Networks[0].Params["psk"] != "\"12345678\"" {
		t.Errorf("Unexpected network 0 psk: %s", cfg.Networks[0].Params["psk"])
	}
	if cfg.Networks[1].Params["ssid"] != "\"Guest\"" {
		t.Errorf("Unexpected network 1 ssid: %s", cfg.Networks[1].Params["ssid"])
	}

	// Now modify and rebuild
	cfg.GlobalParams["country"] = "TW"
	cfg.Networks[0].Params["freq_list"] = "5180 5200"
	cfg.Networks[0].Params["disable_eht"] = "1"
	delete(cfg.Networks[0].Params, "priority")

	output := rebuildConfig(input, cfg)

	// Check output
	if !strings.Contains(output, "country=TW") {
		t.Errorf("Expected country=TW in output:\n%s", output)
	}
	if !strings.Contains(output, "freq_list=5180 5200") {
		t.Errorf("Expected freq_list inside output:\n%s", output)
	}
	if !strings.Contains(output, "disable_eht=1") {
		t.Errorf("Expected disable_eht=1 inside output:\n%s", output)
	}
	if strings.Contains(output, "priority=1") {
		t.Errorf("priority=1 should have been deleted:\n%s", output)
	}

	// Parse rebuilt output to ensure round-trip integrity
	reparsed := parseConfig(output)
	if reparsed.GlobalParams["country"] != "TW" {
		t.Errorf("Roundtrip failed for country: %s", reparsed.GlobalParams["country"])
	}
	if len(reparsed.Networks) != 2 {
		t.Fatalf("Roundtrip expected 2 networks, got %d", len(reparsed.Networks))
	}
	if reparsed.Networks[0].Params["freq_list"] != "5180 5200" {
		t.Errorf("Roundtrip freq_list failed: %s", reparsed.Networks[0].Params["freq_list"])
	}
	if reparsed.Networks[0].Params["disable_eht"] != "1" {
		t.Errorf("Roundtrip disable_eht failed: %s", reparsed.Networks[0].Params["disable_eht"])
	}
	if _, exists := reparsed.Networks[0].Params["priority"]; exists {
		t.Errorf("Roundtrip priority should not exist")
	}
}

func TestParseConfigAndRebuild_EdgeCases(t *testing.T) {
	// Case 1: spaces around =, spaces around network = {, comments after }
	input := `ctrl_interface = /var/run/wpa_supplicant
update_config = 1

network = {
	ssid = "EdgeAP"
	psk = "password"
}
`
	cfg := parseConfig(input)
	if len(cfg.Networks) != 1 {
		t.Fatalf("Expected 1 network, got %d", len(cfg.Networks))
	}
	if cfg.Networks[0].Params["ssid"] != "\"EdgeAP\"" {
		t.Errorf("Expected ssid=\"EdgeAP\", got: %q", cfg.Networks[0].Params["ssid"])
	}
	if cfg.GlobalParams["update_config"] != "1" {
		t.Errorf("Expected update_config=\"1\", got: %q", cfg.GlobalParams["update_config"])
	}

	// Case 2: Deduplication within network and global
	dedupInput := `ctrl_interface=/var/run/wpa_supplicant
update_config=0
update_config=1

network={
	ssid="MyAP"
	priority=2
	priority=5
}
`
	dedupCfg := parseConfig(dedupInput)
	if dedupCfg.GlobalParams["update_config"] != "1" {
		t.Errorf("Expected last value update_config=1, got: %s", dedupCfg.GlobalParams["update_config"])
	}
	if dedupCfg.Networks[0].Params["priority"] != "5" {
		t.Errorf("Expected last value priority=5, got: %s", dedupCfg.Networks[0].Params["priority"])
	}
	dedupOutput := rebuildConfig(dedupInput, dedupCfg)
	if strings.Count(dedupOutput, "update_config=") != 1 {
		t.Errorf("Expected exactly one update_config in rebuilt output:\n%s", dedupOutput)
	}
	if strings.Count(dedupOutput, "priority=") != 1 {
		t.Errorf("Expected exactly one priority in rebuilt output:\n%s", dedupOutput)
	}

	// Case 3: Empty original file, adding a new network
	emptyInput := `ctrl_interface=/var/run/wpa_supplicant`
	emptyCfg := parseConfig(emptyInput)
	emptyCfg.Networks = append(emptyCfg.Networks, &WpaNetwork{
		Params: map[string]string{
			"ssid": "\"TestNew\"",
			"psk":  "\"secret123\"",
		},
	})
	emptyOutput := rebuildConfig(emptyInput, emptyCfg)
	if !strings.Contains(emptyOutput, "network={\n\tssid=\"TestNew\"\n") && !strings.Contains(emptyOutput, "network={\n\tpsk=\"secret123\"\n") {
		t.Errorf("Expected new network block added:\n%s", emptyOutput)
	}
}

func TestWpaEditor_BwLimit(t *testing.T) {
	input := `ctrl_interface=/var/run/wpa_supplicant
network={
	ssid="TestAP"
	psk="12345678"
}
`
	cfg := parseConfig(input)
	cfg.Networks[0].Params["disable_ht40"] = "1"

	output := rebuildConfig(input, cfg)
	if !strings.Contains(output, "disable_ht40=1") {
		t.Fatalf("Expected disable_ht40=1 in output:\n%s", output)
	}

	reparsed := parseConfig(output)
	if reparsed.Networks[0].Params["disable_ht40"] != "1" {
		t.Fatalf("Expected reparsed disable_ht40=1, got: %s", reparsed.Networks[0].Params["disable_ht40"])
	}

	// Delete and rebuild
	delete(reparsed.Networks[0].Params, "disable_ht40")
	clearedOutput := rebuildConfig(output, reparsed)
	if strings.Contains(clearedOutput, "disable_ht40") {
		t.Fatalf("Expected disable_ht40 to be deleted from output:\n%s", clearedOutput)
	}
}

