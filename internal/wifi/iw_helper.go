package wifi

import (
	"fmt"
	"os/exec"
	"strings"
)

// ShowInterfaceDetail displays detailed interface information using 'iw'
func ShowInterfaceDetail(iface string) {
	// Execute 'iw dev <iface> info'
	cmd := exec.Command("iw", "dev", iface, "info")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse Channel and Width
		// Example: channel 4 (2427 MHz), width: 20 MHz, center1: 2427 MHz
		if strings.HasPrefix(line, "channel ") {
			fmt.Printf("  頻道:     %s\n", line)
		}

		// Parse TxPower
		// Example: txpower 18.00 dBm
		if strings.HasPrefix(line, "txpower ") {
			fmt.Printf("  發射功率: %s\n", strings.TrimPrefix(line, "txpower "))
		}
	}
}
