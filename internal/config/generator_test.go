package config

import (
	"strings"
	"testing"

	"qwifitesttool/internal/models"
)

func TestGenerateWpaSupplicantConfig_WithXmlNetwork(t *testing.T) {
	// Simulate FGH200M module with network_ parameters in StationConfig
	module := &models.WiFiModule{
		ID:          "quectel_fgh200m",
		DisplayName: "Quectel FGH200M (Morse Micro MM8108)",
		StationConfig: []models.Param{
			{Key: "ctrl_interface", Value: "/var/run/wpa_supplicant_s1g"},
			{Key: "pmf", Value: "2"},
			{Key: "sae_pwe", Value: "1"},
			{Key: "network_ssid", Value: "sdm108a-b9bf"},
			{Key: "network_key_mgmt", Value: "SAE"},
			{Key: "network_sae_password", Value: "sdm108a@wifi"},
		},
	}

	// Case: Calling with empty ssid and password (as done by connector.go on initial setup)
	content := GenerateWpaSupplicantConfig(module, "/fallback/socket", "", "")

	// 1. Verify global params
	if !strings.Contains(content, "ctrl_interface=/var/run/wpa_supplicant_s1g\n") {
		t.Errorf("Expected XML ctrl_interface to take precedence, got:\n%s", content)
	}
	if !strings.Contains(content, "pmf=2\n") || !strings.Contains(content, "sae_pwe=1\n") {
		t.Errorf("Expected pmf and sae_pwe global params, got:\n%s", content)
	}

	// 2. Verify network block generated from XML
	if !strings.Contains(content, "network={\n") {
		t.Fatalf("Expected network block to be generated from XML network_ params, but not found in:\n%s", content)
	}
	if !strings.Contains(content, "ssid=\"sdm108a-b9bf\"") {
		t.Errorf("Expected quoted ssid=\"sdm108a-b9bf\", got:\n%s", content)
	}
	if !strings.Contains(content, "sae_password=\"sdm108a@wifi\"") {
		t.Errorf("Expected quoted sae_password=\"sdm108a@wifi\", got:\n%s", content)
	}
	if !strings.Contains(content, "key_mgmt=SAE") {
		t.Errorf("Expected key_mgmt=SAE, got:\n%s", content)
	}
	if !strings.Contains(content, "ieee80211w=2") {
		t.Errorf("Expected ieee80211w=2 mandatory for SAE, got:\n%s", content)
	}
	if !strings.Contains(content, "scan_ssid=1") {
		t.Errorf("Expected scan_ssid=1, got:\n%s", content)
	}
}

func TestGenerateWpaSupplicantConfig_WithoutXmlNetwork(t *testing.T) {
	// Simulate standard module without network_ parameters in XML
	module := &models.WiFiModule{
		ID:          "quectel_fcm360w",
		DisplayName: "Quectel FCM360W",
		StationConfig: []models.Param{
			{Key: "ctrl_interface", Value: "/var/run/wpa_supplicant"},
			{Key: "update_config", Value: "1"},
			{Key: "country", Value: "CN"},
		},
	}

	// Case 1: Empty args -> no network block
	content := GenerateWpaSupplicantConfig(module, "", "", "")
	if strings.Contains(content, "network={") {
		t.Errorf("Expected NO network block when neither args nor XML specify a network, got:\n%s", content)
	}
	if !strings.Contains(content, "country=CN\n") || !strings.Contains(content, "update_config=1\n") {
		t.Errorf("Expected global params country and update_config, got:\n%s", content)
	}

	// Case 2: Explicit args passed -> network block generated
	contentWithArgs := GenerateWpaSupplicantConfig(module, "", "HomeWiFi", "secretpass")
	if !strings.Contains(contentWithArgs, "network={\n") {
		t.Fatalf("Expected network block when ssid is provided via args, got:\n%s", contentWithArgs)
	}
	if !strings.Contains(contentWithArgs, "ssid=\"HomeWiFi\"") {
		t.Errorf("Expected ssid=\"HomeWiFi\", got:\n%s", contentWithArgs)
	}
	if !strings.Contains(contentWithArgs, "psk=\"secretpass\"") {
		t.Errorf("Expected psk=\"secretpass\", got:\n%s", contentWithArgs)
	}
}
