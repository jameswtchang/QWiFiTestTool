package wifi

import (
	"os"
	"strings"
)

// configHasNetwork checks if the config file contains a valid network block for auto-connect
func (c *Connector) configHasNetwork() bool {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return false
	}

	content := string(data)

	// Look for "network={" or "network ="
	if !strings.Contains(content, "network={") && !strings.Contains(content, "network =") {
		return false
	}

	// Ensure there is at least one real network (not just DummyAP_For_Test placeholder)
	hasRealNetwork := false
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == "ssid" {
			val := strings.TrimSpace(parts[1])
			if !strings.Contains(val, "DummyAP_For_Test") && val != `""` && val != "" {
				hasRealNetwork = true
				break
			}
		}
	}

	return hasRealNetwork
}
