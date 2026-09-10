package config

import (
	"fmt"
	"strconv"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/ui"
)

// BandPreset represents a predefined band, mode, and bandwidth combination
type BandPreset struct {
	Label     string // e.g. "WiFi 7 (802.11be) - 160 MHz"
	Band      string // "2.4G", "5G", "6G", "sub-GHz"
	Mode      string // "be", "ax", "ac", "n", "ah"
	Bandwidth int    // 20, 40, 80, 160, 320
}

// BandInfo represents a band and its highest supported capability
type BandInfo struct {
	Band     string
	Display  string
	MaxLabel string // e.g. "最高: be-160M"
}

// getAllPresets returns all defined presets in order of preference
func getAllPresets() []BandPreset {
	return []BandPreset{
		// 6 GHz presets
		{"WiFi 7 (802.11be) - 320 MHz", "6G", "be", 320},
		{"WiFi 7 (802.11be) - 160 MHz", "6G", "be", 160},
		{"WiFi 7 (802.11be) - 80 MHz", "6G", "be", 80},
		{"WiFi 6 (802.11ax) - 160 MHz", "6G", "ax", 160},
		{"WiFi 6 (802.11ax) - 80 MHz", "6G", "ax", 80},
		{"WiFi 6 (802.11ax) - 40 MHz", "6G", "ax", 40},
		{"WiFi 6 (802.11ax) - 20 MHz", "6G", "ax", 20},

		// 5 GHz presets
		{"WiFi 7 (802.11be) - 160 MHz", "5G", "be", 160},
		{"WiFi 7 (802.11be) - 80 MHz", "5G", "be", 80},
		{"WiFi 6 (802.11ax) - 160 MHz", "5G", "ax", 160},
		{"WiFi 6 (802.11ax) - 80 MHz", "5G", "ax", 80},
		{"WiFi 6 (802.11ax) - 40 MHz", "5G", "ax", 40},
		{"WiFi 6 (802.11ax) - 20 MHz", "5G", "ax", 20},
		{"WiFi 5 (802.11ac) - 80 MHz", "5G", "ac", 80},
		{"WiFi 5 (802.11ac) - 40 MHz", "5G", "ac", 40},
		{"WiFi 4 (802.11n)  - 40 MHz", "5G", "n", 40},
		{"WiFi 4 (802.11n)  - 20 MHz", "5G", "n", 20},

		// 2.4 GHz presets
		{"WiFi 7 (802.11be) - 40 MHz", "2.4G", "be", 40},
		{"WiFi 7 (802.11be) - 20 MHz", "2.4G", "be", 20},
		{"WiFi 6 (802.11ax) - 40 MHz", "2.4G", "ax", 40},
		{"WiFi 6 (802.11ax) - 20 MHz", "2.4G", "ax", 20},
		{"WiFi 4 (802.11n)  - 40 MHz", "2.4G", "n", 40},
		{"WiFi 4 (802.11n)  - 20 MHz", "2.4G", "n", 20},

		// Sub-GHz presets
		{"WiFi HaLow (802.11ah) - 2 MHz", "sub-GHz", "ah", 2},
	}
}

// filterPresets filters presets based on module capabilities and band
func filterPresets(caps *models.APCapabilities, band string) []BandPreset {
	all := getAllPresets()
	var filtered []BandPreset

	for _, p := range all {
		if p.Band != band {
			continue
		}
		if caps != nil {
			if !caps.HasBand(p.Band) {
				continue
			}
			if !caps.HasMode(p.Mode) {
				continue
			}
			if caps.MaxBandwidth > 0 && p.Bandwidth > caps.MaxBandwidth {
				continue
			}
		}
		filtered = append(filtered, p)
	}

	return filtered
}

// getAvailableBands returns bands supported by the module with highest capability notes
func getAvailableBands(caps *models.APCapabilities) []BandInfo {
	bandOrder := []struct {
		band    string
		display string
	}{
		{"2.4G", "2.4 GHz"},
		{"5G", "5 GHz"},
		{"6G", "6 GHz"},
		{"sub-GHz", "Sub-GHz (802.11ah)"},
	}

	var result []BandInfo
	for _, b := range bandOrder {
		if caps != nil && !caps.HasBand(b.band) {
			continue
		}

		presets := filterPresets(caps, b.band)
		if len(presets) == 0 {
			continue
		}

		// First preset is the highest capability
		highest := presets[0]
		maxLabel := fmt.Sprintf("%s-%dM", highest.Mode, highest.Bandwidth)

		result = append(result, BandInfo{
			Band:     b.band,
			Display:  b.display,
			MaxLabel: maxLabel,
		})
	}

	return result
}

// calcPresetCenterChannel calculates center channel for display
func calcPresetCenterChannel(band string, bw int, primaryCh int) int {
	if primaryCh <= 0 {
		return 0
	}

	if band == "6G" {
		if bw == 320 {
			centers := []int{31, 63, 95, 127, 159, 191}
			for _, c := range centers {
				if primaryCh >= c-30 && primaryCh <= c+30 {
					return c
				}
			}
		} else if bw == 160 {
			centers := []int{15, 47, 79, 111, 143, 175, 207}
			for _, c := range centers {
				if primaryCh >= c-14 && primaryCh <= c+14 {
					return c
				}
			}
		} else if bw == 80 {
			centers := []int{7, 23, 39, 55, 71, 87, 103, 119, 135, 151, 167, 183, 199, 215, 231}
			for _, c := range centers {
				if primaryCh >= c-6 && primaryCh <= c+6 {
					return c
				}
			}
		} else if bw == 40 {
			centers := []int{3, 11, 19, 27, 35, 43, 51, 59, 67, 75, 83, 91, 99, 107, 115, 123, 131, 139, 147, 155, 163, 171, 179, 187, 195, 203, 211, 219, 227}
			for _, c := range centers {
				if primaryCh >= c-2 && primaryCh <= c+2 {
					return c
				}
			}
		}
	} else if band == "5G" {
		if bw == 160 {
			centers := []int{50, 114, 163}
			for _, c := range centers {
				if primaryCh >= c-14 && primaryCh <= c+14 {
					return c
				}
			}
		} else if bw == 80 {
			centers := []int{42, 58, 106, 122, 138, 155}
			for _, c := range centers {
				if primaryCh >= c-6 && primaryCh <= c+6 {
					return c
				}
			}
		}
	}

	return 0
}

// getRecommendedChannel returns the recommended channel for a band/bandwidth
// For 6GHz with bandwidth > 20MHz, this returns the center channel ID (matching country_channel_lookup.json)
func (e *APConfigEditor) getRecommendedChannel(band string, bw int) int {
	channels := e.channelDB.LookupChannels(e.params.CountryCode, e.params.APType, band, bw)
	if len(channels) == 0 {
		// Fallback defaults if not in lookup DB
		switch band {
		case "6G":
			switch bw {
			case 320:
				return 31
			case 160:
				return 47
			case 80:
				return 39
			case 40:
				return 35
			default:
				return 37 // 20M PSC
			}
		case "5G":
			return 36
		case "2.4G":
			return 6
		case "sub-GHz":
			return 40
		default:
			return 1
		}
	}

	// 6G: Prefer center channel that contains primary PSC channel 37
	if band == "6G" {
		for _, ch := range channels {
			if bw == 20 && ch == 37 {
				return 37
			}
			if bw == 40 && ch == 35 { // contains 37
				return 35
			}
			if bw == 80 && ch == 39 { // contains 37
				return 39
			}
			if bw == 160 && ch == 47 { // contains 37
				return 47
			}
			if bw == 320 && ch == 31 { // contains 37
				return 31
			}
		}
		return channels[0]
	}

	// 5G: Prefer 36 if available
	if band == "5G" {
		for _, ch := range channels {
			if ch == 36 {
				return 36
			}
		}
	}

	// 2.4G: Prefer 6 if available
	if band == "2.4G" {
		for _, ch := range channels {
			if ch == 6 {
				return 6
			}
		}
	}

	return channels[0]
}

// runQuickPresetWizard runs the interactive preset configuration wizard
func (e *APConfigEditor) runQuickPresetWizard() bool {
	caps := e.module.APConfig.Capabilities

	fmt.Println("\n" + ui.Cyan + ui.Bold + "╔═══════════════════════════════════════════════════╗" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "║   " + i18n.T("qp.title") + "   ║" + ui.Reset)
	fmt.Println(ui.Cyan + ui.Bold + "╚═══════════════════════════════════════════════════╝" + ui.Reset)

	// Step 1: Select Band
	availableBands := getAvailableBands(caps)
	if len(availableBands) == 0 {
		fmt.Println(ui.Red + i18n.T("qp.no_bands") + ui.Reset)
		return false
	}

	fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.T("qp.select_band") + ui.Reset)
	for i, b := range availableBands {
		fmt.Printf(ui.Cyan+ui.Bold+"  [%d]"+ui.Reset+" %-12s "+ui.Green+"("+i18n.T("qp.max_prefix")+": %s)"+ui.Reset+"\n",
			i+1, b.Display, b.MaxLabel)
	}
	fmt.Println(ui.Cyan + ui.Bold + "  [0]" + ui.Reset + " " + i18n.T("common.back"))
	fmt.Print(ui.Yellow + ui.Bold + i18n.T("common.select_num") + ui.Reset)

	bandInput := e.readLine()
	if bandInput == "0" || bandInput == "" {
		return false
	}

	bandIdx, err := strconv.Atoi(bandInput)
	if err != nil || bandIdx < 1 || bandIdx > len(availableBands) {
		fmt.Println(ui.Red + i18n.T("common.invalid_sel") + ui.Reset)
		return false
	}

	selectedBand := availableBands[bandIdx-1]

	// Step 2: Select Preset for this band
	presets := filterPresets(caps, selectedBand.Band)
	if len(presets) == 0 {
		fmt.Println(ui.Red + i18n.T("qp.no_presets") + ui.Reset)
		return false
	}

	fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.Tf("qp.select_preset", selectedBand.Display) + ui.Reset)
	for i, p := range presets {
		recCh := e.getRecommendedChannel(p.Band, p.Bandwidth)
		var chInfo string

		if p.Band == "6G" {
			if p.Bandwidth > 20 {
				primaryCh := CalcPrimaryChannel6G(recCh, p.Bandwidth)
				chInfo = fmt.Sprintf("[%s: %d, Primary: %d (PSC)]",
					i18n.T("qp.center_channel"), recCh, primaryCh)
			} else {
				chInfo = fmt.Sprintf("[%s: %d (PSC)]", i18n.T("qp.rec_channel"), recCh)
			}
		} else {
			centerCh := calcPresetCenterChannel(p.Band, p.Bandwidth, recCh)
			chInfo = fmt.Sprintf("[%s: %d", i18n.T("qp.rec_channel"), recCh)
			if centerCh > 0 && centerCh != recCh {
				chInfo += fmt.Sprintf(", %s: %d", i18n.T("qp.center_channel"), centerCh)
			}
			chInfo += "]"
		}

		fmt.Printf(ui.Cyan+ui.Bold+"  [%2d]"+ui.Reset+" %-32s "+ui.Yellow+"%s"+ui.Reset+"\n",
			i+1, p.Label, chInfo)
	}
	fmt.Println(ui.Cyan + ui.Bold + "  [ 0]" + ui.Reset + " " + i18n.T("common.back"))
	fmt.Print(ui.Yellow + ui.Bold + i18n.T("common.select_num") + ui.Reset)

	presetInput := e.readLine()
	if presetInput == "0" || presetInput == "" {
		return false
	}

	presetIdx, err := strconv.Atoi(presetInput)
	if err != nil || presetIdx < 1 || presetIdx > len(presets) {
		fmt.Println(ui.Red + i18n.T("common.invalid_sel") + ui.Reset)
		return false
	}

	chosen := presets[presetIdx-1]
	recChannel := e.getRecommendedChannel(chosen.Band, chosen.Bandwidth)

	// Step 3: Apply parameters (Preserve SSID, Password, CountryCode)
	e.params.Band = chosen.Band
	e.params.Mode = chosen.Mode
	e.params.Bandwidth = chosen.Bandwidth
	e.params.Channel = recChannel

	// 6GHz strictly requires WPA3-SAE
	if chosen.Band == "6G" {
		e.params.Security = "WPA3"
		if e.params.Password == "" {
			e.params.Password = "12345678"
		}
	}

	e.refreshChannels()

	if chosen.Band == "6G" && chosen.Bandwidth > 20 {
		primaryCh := CalcPrimaryChannel6G(recChannel, chosen.Bandwidth)
		fmt.Printf("\n"+ui.Green+ui.Bold+"✅ "+i18n.Tf("qp.applied_success",
			chosen.Band, chosen.Mode, chosen.Bandwidth, recChannel)+
			fmt.Sprintf(" (Primary: %d)"+ui.Reset+"\n\n", primaryCh))
	} else {
		fmt.Printf("\n"+ui.Green+ui.Bold+"✅ "+i18n.Tf("qp.applied_success",
			chosen.Band, chosen.Mode, chosen.Bandwidth, recChannel)+ui.Reset+"\n\n")
	}

	return true
}
