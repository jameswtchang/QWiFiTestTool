package wifi

import (
	"fmt"
	"os/exec"
	"strings"

	"qwifitesttool/internal/i18n"
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
			if strings.Contains(line, "width: unknown") {
				// Old 'iw' tool doesn't recognize NL80211_CHAN_WIDTH_320 (value 13).
				// Verify via center1 frequency (6105, 6265, 6425 MHz are exactly the 320 MHz centers in 6GHz).
				if strings.Contains(line, "center1: 6105") || strings.Contains(line, "center1: 6265") || strings.Contains(line, "center1: 6425") || strings.Contains(line, "center1: 6585") {
					line = strings.ReplaceAll(line, "width: unknown", "width: 320 MHz (802.11be EHT)")
				} else {
					// Fallback indication if it's an unrecognized width enum
					line = strings.ReplaceAll(line, "width: unknown", "width: 320 MHz (802.11be / Unknown)")
				}
			}
			fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("iw.channel"), 16), line)
		}

		// Parse TxPower
		// Example: txpower 18.00 dBm or buggy txpower 42949572.96 dBm
		if strings.HasPrefix(line, "txpower ") {
			powerStr := strings.TrimPrefix(line, "txpower ")
			if strings.Contains(powerStr, "42949") {
				// Driver returned an uninitialized/negative value that wrapped around to 2^32
				fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("iw.txpower"), 16), i18n.T("iw.txpower_unknown"))
			} else {
				fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("iw.txpower"), 16), powerStr)
			}
		}
	}
}
