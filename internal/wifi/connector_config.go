package wifi

import (
	"os"
	"strings"
)

// configHasNetwork checks if the config file contains a network block
func (c *Connector) configHasNetwork() bool {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return false
	}

	content := string(data)

	// Look for "network={" or "network ="
	// Simple check - could be improved with proper parsing
	return strings.Contains(content, "network={") ||
		strings.Contains(content, "network =")
}
