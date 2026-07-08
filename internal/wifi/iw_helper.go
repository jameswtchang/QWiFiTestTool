package wifi

import (
	"fmt"
	"os/exec"
	"strings"

	"qwifitesttool/internal/ui"
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
			fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("頻道:", 10), line)
		}

		// Parse TxPower
		// Example: txpower 18.00 dBm or buggy txpower 42949572.96 dBm
		if strings.HasPrefix(line, "txpower ") {
			powerStr := strings.TrimPrefix(line, "txpower ")
			if strings.Contains(powerStr, "42949") {
				// Driver returned an uninitialized/negative value that wrapped around to 2^32
				fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel("發射功率:", 10), "未知 (Driver 未提供)")
			} else {
				fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel("發射功率:", 10), powerStr)
			}
		}
	}
}
