package wifi

import (
	"fmt"
	"os"
	"strings"
)

// updateWpaSupplicantConfig updates the network block in wpa_supplicant.conf
func updateWpaSupplicantConfig(configPath, ssid, password string) error {
	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		// If file doesn't exist, create a basic one
		data = []byte("")
	}

	content := string(data)

	// Remove existing network block
	content = removeNetworkBlock(content)

	// Build new network block
	networkBlock := buildNetworkBlock(ssid, password)

	// Combine config
	newConfig := content + "\n" + networkBlock

	// Write back
	fmt.Printf("[INFO] 更新配置文件: %s\n", configPath)
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		return fmt.Errorf("寫入配置文件失敗: %w", err)
	}

	fmt.Println("[SUCCESS] 配置文件已更新")
	return nil
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

		// Keep non-network lines
		if trimmed != "" {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// buildNetworkBlock creates a network configuration block
func buildNetworkBlock(ssid, password string) string {
	var block strings.Builder

	block.WriteString("network={\n")
	block.WriteString(fmt.Sprintf(" ssid=\"%s\"\n", ssid))

	if password != "" {
		block.WriteString(fmt.Sprintf(" psk=\"%s\"\n", password))
	} else {
		block.WriteString(" key_mgmt=NONE\n")
	}

	block.WriteString("}\n")

	return block.String()
}

// printConfigFile displays the updated config file
func printConfigFile(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("[ERROR] 無法讀取配置文件: %v\n", err)
		return
	}

	fmt.Println("\n========== 配置文件內容 ==========")
	fmt.Println(string(data))
	fmt.Println("==================================")
}
