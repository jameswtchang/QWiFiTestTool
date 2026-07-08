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
	var networkKeyMgmt string
	networkParams := make(map[string]string)

	for key, value := range paramsMap {
		// Skip ctrl_interface if already set
		if key == "ctrl_interface" && ctrlInterface != "" {
			continue
		}
		// Separate network-specific parameters (network_ prefix)
		if strings.HasPrefix(key, "network_") {
			paramKey := strings.TrimPrefix(key, "network_")
			if paramKey == "key_mgmt" {
				networkKeyMgmt = value
			} else {
				networkParams[paramKey] = value
			}
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
			if networkKeyMgmt == "SAE" {
				sb.WriteString(fmt.Sprintf("    sae_password=\"%s\"\n", password))
				sb.WriteString(fmt.Sprintf("    key_mgmt=%s\n", networkKeyMgmt))
				sb.WriteString("    ieee80211w=2\n")
			} else {
				sb.WriteString(fmt.Sprintf("    psk=\"%s\"\n", password))
				if networkKeyMgmt != "" {
					sb.WriteString(fmt.Sprintf("    key_mgmt=%s\n", networkKeyMgmt))
				}
			}
		} else {
			sb.WriteString("    key_mgmt=NONE\n")
		}

		// Write remaining network params from XML
		for k, v := range networkParams {
			if k == "ssid" || k == "sae_password" || k == "psk" {
				continue
			}
			sb.WriteString(fmt.Sprintf("    %s=%s\n", k, v))
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
	if band == "5G" || band == "5g" || band == "6G" || band == "6g" {
		sb.WriteString("hw_mode=a\n")
		if module.APConfig.APProfile != nil && module.APConfig.APProfile.Channel != 0 {
			sb.WriteString(fmt.Sprintf("channel=%d\n", module.APConfig.APProfile.Channel))
		} else if band == "6G" || band == "6g" {
			sb.WriteString("channel=37\n")
		} else {
			sb.WriteString("channel=36\n")
		}
	} else if band == "sub-GHz" {
		sb.WriteString("hw_mode=a\n")
		sb.WriteString(fmt.Sprintf("channel=%d\n", module.APConfig.APProfile.Channel))
	} else {
		sb.WriteString("hw_mode=g\n")
		sb.WriteString("channel=6\n")
	}

	// Security settings fallback to APProfile
	if password == "" && module.APConfig.APProfile != nil {
		password = module.APConfig.APProfile.Password
	}

	if password != "" {
		sb.WriteString("wpa=2\n")
		sb.WriteString("rsn_pairwise=CCMP\n")
		if module.APConfig.APProfile != nil && module.APConfig.APProfile.Security == "WPA3" {
			sb.WriteString(fmt.Sprintf("sae_password=\"%s\"\n", password))
			sb.WriteString("wpa_key_mgmt=SAE\n")
			sb.WriteString("ieee80211w=2\n")
		} else {
			sb.WriteString(fmt.Sprintf("wpa_passphrase=%s\n", password))
			sb.WriteString("wpa_key_mgmt=WPA-PSK\n")
		}
	}

	// Additional parameters from XML
	var allParams []models.Param
	allParams = append(allParams, module.APConfig.FixedParams...)
	allParams = append(allParams, module.APConfig.Params...)

	paramsMap := models.ParamsToMap(allParams)
	for key, value := range paramsMap {
		// Skip dnsmasq parameters in hostapd config
		if strings.HasPrefix(key, "dnsmasq_") {
			continue
		}

		// Skip parameters we've already set
		if key == "interface" || key == "driver" || key == "ssid" ||
			key == "hw_mode" || key == "channel" || key == "wpa" ||
			key == "rsn_pairwise" || key == "sae_password" || key == "wpa_passphrase" || key == "wpa_key_mgmt" || key == "ieee80211w" {
			continue
		}
		if (band == "6G" || band == "6g") && (key == "ieee80211n" || key == "ieee80211ac" || key == "ht_capab" || key == "vht_capab" || key == "vht_oper_chwidth" || key == "vht_oper_centr_freq_seg0_idx") {
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

	// Extract custom dnsmasq params
	var allParams []models.Param
	allParams = append(allParams, module.APConfig.FixedParams...)
	allParams = append(allParams, module.APConfig.Params...)

	paramsMap := models.ParamsToMap(allParams)
	dnsmasqParams := make(map[string]string)
	for k, v := range paramsMap {
		if strings.HasPrefix(k, "dnsmasq_") {
			dnsmasqParams[strings.TrimPrefix(k, "dnsmasq_")] = v
		}
	}

	user := "root"
	if u, ok := dnsmasqParams["user"]; ok {
		user = u
		delete(dnsmasqParams, "user")
	}

	listenAddress := cfg.GatewayIP
	if la, ok := dnsmasqParams["listen_address"]; ok {
		listenAddress = la
		delete(dnsmasqParams, "listen_address")
	}

	server := cfg.DNSServer
	if s, ok := dnsmasqParams["server"]; ok {
		server = s
		delete(dnsmasqParams, "server")
	}

	bindInterfaces := true
	if bi, ok := dnsmasqParams["bind_interfaces"]; ok {
		if bi == "0" || bi == "false" {
			bindInterfaces = false
		}
		delete(dnsmasqParams, "bind_interfaces")
	}

	// Header comment
	sb.WriteString("# dnsmasq configuration for AP mode\n")
	sb.WriteString(fmt.Sprintf("# Generated for module: %s\n", module.DisplayName))
	sb.WriteString("# Generated at: " + getCurrentTime() + "\n\n")

	// Interface and binding
	sb.WriteString(fmt.Sprintf("user=%s\n", user))
	sb.WriteString(fmt.Sprintf("interface=%s\n", cfg.Interface))
	sb.WriteString(fmt.Sprintf("listen-address=%s\n", listenAddress))
	if bindInterfaces {
		sb.WriteString("bind-interfaces\n")
	}

	// DHCP settings
	sb.WriteString("\n# DHCP settings\n")
	sb.WriteString(fmt.Sprintf("dhcp-range=%s\n", cfg.DHCPRange))
	sb.WriteString(fmt.Sprintf("dhcp-option=option:router,%s\n", cfg.GatewayIP))
	sb.WriteString(fmt.Sprintf("dhcp-option=option:netmask,%s\n", cfg.SubnetMask))
	sb.WriteString(fmt.Sprintf("dhcp-option=option:dns-server,%s\n", cfg.DNSServer))

	// DNS settings
	sb.WriteString("\n# DNS settings\n")
	sb.WriteString(fmt.Sprintf("server=%s\n", server))

	// Custom dnsmasq params
	if len(dnsmasqParams) > 0 {
		sb.WriteString("\n# Custom settings\n")
		for k, v := range dnsmasqParams {
			k = strings.ReplaceAll(k, "_", "-")
			sb.WriteString(fmt.Sprintf("%s=%s\n", k, v))
		}
	}

	// Logging
	sb.WriteString("\n# Logging\n")
	sb.WriteString("log-queries\n")
	sb.WriteString("log-dhcp\n")

	return sb.String()
}
