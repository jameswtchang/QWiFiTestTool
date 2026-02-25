package config

import (
	"fmt"
	"strings"

	"qwifitesttool/internal/models"
)

// GenerateWpaSupplicantConfig generates wpa_supplicant configuration file content
func GenerateWpaSupplicantConfig(module *models.WiFiModule, ctrlInterface, ssid, password string) string {
	var sb strings.Builder

	// Header comment
	sb.WriteString("# wpa_supplicant configuration\n")
	sb.WriteString(fmt.Sprintf("# Generated for module: %s\n", module.DisplayName))
	sb.WriteString("# Generated at: " + getCurrentTime() + "\n\n")

	// Control interface (auto-detected or from XML)
	if ctrlInterface != "" {
		sb.WriteString(fmt.Sprintf("ctrl_interface=%s\n", ctrlInterface))
	}

	// Global parameters from XML
	paramsMap := models.ParamsToMap(module.StationConfig)
	for key, value := range paramsMap {
		// Skip ctrl_interface if already set
		if key == "ctrl_interface" && ctrlInterface != "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	// Add network block if SSID provided
	if ssid != "" {
		sb.WriteString("\n# Network configuration\n")
		sb.WriteString("network={\n")
		sb.WriteString(fmt.Sprintf("    ssid=\"%s\"\n", ssid))

		if password != "" {
			sb.WriteString(fmt.Sprintf("    psk=\"%s\"\n", password))
		} else {
			sb.WriteString("    key_mgmt=NONE\n")
		}

		sb.WriteString("    scan_ssid=1\n")
		sb.WriteString("}\n")
	}

	return sb.String()
}

// GenerateHostapdConfig generates hostapd configuration file content
func GenerateHostapdConfig(module *models.WiFiModule, ssid, password, band string) string {
	var sb strings.Builder

	// Header comment
	sb.WriteString("# hostapd configuration\n")
	sb.WriteString(fmt.Sprintf("# Generated for module: %s\n", module.DisplayName))
	sb.WriteString("# Generated at: " + getCurrentTime() + "\n\n")

	// Interface and driver (from Kernel config)
	sb.WriteString(fmt.Sprintf("interface=%s\n", module.Kernel.GetInterface()))
	sb.WriteString(fmt.Sprintf("driver=%s\n", module.Kernel.GetDriverBackend()))

	// User-specified values
	sb.WriteString(fmt.Sprintf("ssid=%s\n", ssid))

	// Band-specific settings
	if band == "5G" || band == "5g" {
		sb.WriteString("hw_mode=a\n")
		sb.WriteString("channel=36\n")
	} else {
		sb.WriteString("hw_mode=g\n")
		sb.WriteString("channel=6\n")
	}

	// Security settings
	if password != "" {
		sb.WriteString("wpa=2\n")
		sb.WriteString(fmt.Sprintf("wpa_passphrase=%s\n", password))
		sb.WriteString("wpa_key_mgmt=WPA-PSK\n")
		sb.WriteString("rsn_pairwise=CCMP\n")
	}

	// Additional parameters from XML
	paramsMap := models.ParamsToMap(module.APConfig.Params)
	for key, value := range paramsMap {
		// Skip parameters we've already set
		if key == "interface" || key == "driver" || key == "ssid" ||
			key == "hw_mode" || key == "channel" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	return sb.String()
}

// getCurrentTime returns current timestamp as string
func getCurrentTime() string {
	return fmt.Sprintf("%v", "AUTO_GENERATED")
}

// DnsmasqConfig holds dnsmasq configuration parameters
type DnsmasqConfig struct {
	Interface  string // e.g., wlan0
	DHCPRange  string // e.g., 192.168.50.10,192.168.50.100,12h
	GatewayIP  string // e.g., 192.168.50.1
	SubnetMask string // e.g., 255.255.255.0
	LeaseTime  string // e.g., 12h
	DNSServer  string // e.g., 8.8.8.8
}

// DefaultDnsmasqConfig returns default dnsmasq configuration
func DefaultDnsmasqConfig(iface string) DnsmasqConfig {
	return DnsmasqConfig{
		Interface:  iface,
		DHCPRange:  "192.168.50.10,192.168.50.100,12h",
		GatewayIP:  "192.168.50.1",
		SubnetMask: "255.255.255.0",
		LeaseTime:  "12h",
		DNSServer:  "8.8.8.8",
	}
}

// GenerateDnsmasqConfig generates dnsmasq configuration file content
func GenerateDnsmasqConfig(module *models.WiFiModule, cfg DnsmasqConfig) string {
	var sb strings.Builder

	// Header comment
	sb.WriteString("# dnsmasq configuration for AP mode\n")
	sb.WriteString(fmt.Sprintf("# Generated for module: %s\n", module.DisplayName))
	sb.WriteString("# Generated at: " + getCurrentTime() + "\n\n")

	// Interface binding
	sb.WriteString(fmt.Sprintf("interface=%s\n", cfg.Interface))
	sb.WriteString(fmt.Sprintf("bind-interfaces\n"))

	// DHCP settings
	sb.WriteString(fmt.Sprintf("\n# DHCP settings\n"))
	sb.WriteString(fmt.Sprintf("dhcp-range=%s\n", cfg.DHCPRange))
	sb.WriteString(fmt.Sprintf("dhcp-option=option:router,%s\n", cfg.GatewayIP))
	sb.WriteString(fmt.Sprintf("dhcp-option=option:netmask,%s\n", cfg.SubnetMask))
	sb.WriteString(fmt.Sprintf("dhcp-option=option:dns-server,%s\n", cfg.DNSServer))

	// DNS settings
	sb.WriteString(fmt.Sprintf("\n# DNS settings\n"))
	sb.WriteString(fmt.Sprintf("server=%s\n", cfg.DNSServer))

	// Logging
	sb.WriteString(fmt.Sprintf("\n# Logging\n"))
	sb.WriteString("log-queries\n")
	sb.WriteString("log-dhcp\n")

	return sb.String()
}
