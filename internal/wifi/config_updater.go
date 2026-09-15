package wifi

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"qwifitesttool/internal/i18n"
)

// updateWpaSupplicantConfig updates the network block in wpa_supplicant.conf
func updateWpaSupplicantConfig(configPath, ssid, password, flags string) error {
	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		// If file doesn't exist, create a basic one
		data = []byte("")
	}

	content := string(data)

	// Extract existing policy params (freq_list, disable_*) to preserve them across connections
	policies := extractPolicyParams(content)

	// Remove existing network block
	content = removeNetworkBlock(content)

	// Build new network block based on scan flags and preserved policies
	networkBlock := buildNetworkBlock(ssid, password, flags, policies)

	// Combine config
	newConfig := content + "\n" + networkBlock

	// Write back
	fmt.Println(i18n.Tf("cfg.updating", configPath))
	if err := os.WriteFile(configPath, []byte(newConfig), 0600); err != nil {
		exec.Command("mount", "-o", "remount,rw", "/").Run()
		if err2 := os.WriteFile(configPath, []byte(newConfig), 0600); err2 != nil {
			return fmt.Errorf(i18n.T("cfg.write_fail"), err2)
		}
	}

	fmt.Println(i18n.T("cfg.updated"))
	return nil
}

// extractPolicyParams parses policy parameters from existing network blocks (e.g. freq_list, disable_eht)
func extractPolicyParams(content string) map[string]string {
	params := make(map[string]string)
	lines := strings.Split(content, "\n")
	inNetwork := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		noSpace := strings.ReplaceAll(trimmed, " ", "")
		if strings.HasPrefix(noSpace, "network={") {
			inNetwork = true
			continue
		}

		if (trimmed == "}" || strings.HasPrefix(trimmed, "}")) && inNetwork {
			inNetwork = false
			continue
		}

		if inNetwork {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				switch k {
				case "freq_list", "disable_eht", "disable_he", "disable_vht", "scan_ssid":
					params[k] = v
				}
			}
		}
	}
	return params
}

// removeNetworkBlock removes existing network={...} block from config
func removeNetworkBlock(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inNetworkBlock := false
	braceCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if entering network block
		if strings.HasPrefix(trimmed, "network=") || strings.HasPrefix(trimmed, "network {") {
			inNetworkBlock = true
			braceCount = strings.Count(line, "{") - strings.Count(line, "}")
			continue
		}

		if inNetworkBlock {
			braceCount += strings.Count(line, "{") - strings.Count(line, "}")
			if braceCount <= 0 {
				inNetworkBlock = false
			}
			continue
		}

		// Keep non-network lines (including blank lines)
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// deriveKeyMgmt determines key_mgmt value from scan flags
func deriveKeyMgmt(flags string) string {
	upper := strings.ToUpper(flags)

	if strings.Contains(upper, "SAE") {
		return "SAE"
	}
	if strings.Contains(upper, "WPA3") {
		return "SAE"
	}
	if strings.Contains(upper, "WPA2-PSK") || strings.Contains(upper, "RSN-PSK") {
		return "WPA-PSK"
	}
	if strings.Contains(upper, "WPA-PSK") {
		return "WPA-PSK"
	}
	if strings.Contains(upper, "WPA2") || strings.Contains(upper, "WPA") {
		return "WPA-PSK"
	}

	return ""
}

// buildNetworkBlock creates a network configuration block
// flags is the raw flags string from wpa_cli scan_results
func buildNetworkBlock(ssid, password, flags string, policies map[string]string) string {
	var block strings.Builder

	block.WriteString("network={\n")
	block.WriteString(fmt.Sprintf(" ssid=\"%s\"\n", ssid))

	keyMgmt := deriveKeyMgmt(flags)

	if password == "" {
		block.WriteString(" key_mgmt=NONE\n")
	} else {
		if keyMgmt != "" {
			block.WriteString(fmt.Sprintf(" key_mgmt=%s\n", keyMgmt))
		}
		if keyMgmt == "SAE" {
			block.WriteString(fmt.Sprintf(" sae_password=\"%s\"\n", password))
			block.WriteString(" ieee80211w=2\n")
		} else {
			block.WriteString(fmt.Sprintf(" psk=\"%s\"\n", password))
		}
	}

	// Preserve policy constraints (freq_list, disable_*)
	for k, v := range policies {
		block.WriteString(fmt.Sprintf(" %s=%s\n", k, v))
	}

	block.WriteString("}\n")

	return block.String()
}

// printConfigFile displays the updated config file
func printConfigFile(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Println(i18n.Tf("cfg.read_fail", err))
		return
	}

	fmt.Println("\n========== " + i18n.T("cfg.content_title") + " ==========")
	fmt.Println(string(data))
	fmt.Println("==================================")
}
