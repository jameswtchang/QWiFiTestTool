package wifi

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
)

// iwPath resolves the iw binary once ($PATH -> tool dir -> system dirs incl. extracted assets).
// Stock Android usually has no iw in $PATH, so a bare "iw" exec would always fail there.
var iwPath = sync.OnceValue(func() string {
	return sys.FindCommand("iw", "iw")
})

// iwAvailable reports whether the resolved iw binary can actually be executed
func iwAvailable() bool {
	_, err := exec.LookPath(iwPath())
	return err == nil
}

// ShowInterfaceDetail displays detailed interface information using 'iw'
func ShowInterfaceDetail(iface string) {
	// Execute 'iw dev <iface> info'
	cmd := exec.Command(iwPath(), "dev", iface, "info")
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
		// Example: txpower 18.00 dBm, 0.00 dBm (Auto TPC), 63.00 dBm (Regulatory max limit), or buggy 42949572.96 dBm
		if strings.HasPrefix(line, "txpower ") {
			txPower := FormatTxPower(line)
			if strings.Contains(line, "42949") {
				fmt.Printf(ui.Bold+"  %s "+ui.Yellow+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("iw.txpower"), 16), txPower)
			} else {
				fmt.Printf(ui.Bold+"  %s "+ui.Green+"%s"+ui.Reset+"\n", ui.AlignLabel(i18n.T("iw.txpower"), 16), txPower)
			}
		}
	}
}

// FormatTxPower maps raw iw txpower string to a human-readable display string
func FormatTxPower(powerStr string) string {
	powerStr = strings.TrimSpace(strings.TrimPrefix(powerStr, "txpower "))
	if strings.Contains(powerStr, "42949") {
		return i18n.T("iw.txpower_unknown")
	}
	if strings.HasPrefix(powerStr, "0.00") || strings.HasPrefix(powerStr, "0 ") {
		return i18n.T("iw.txpower_auto")
	}
	if strings.HasPrefix(powerStr, "63.00") || strings.HasPrefix(powerStr, "63 ") {
		return i18n.T("iw.txpower_auto_limit")
	}
	return powerStr
}
