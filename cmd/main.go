package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"qwifitesttool/internal/config"
	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
	"qwifitesttool/internal/wifi"
)

// Version defines the application version string
const Version = "V1.03"

func main() {
	// Show banner
	ui.ShowBanner(Version)

	// Language selection: default zh-TW, ask to switch to English
	fmt.Print("Switch to English? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	langInput, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(langInput)) == "y" {
		i18n.SetLang(i18n.EnUS)
		// Reprint banner in English
		fmt.Println()
		ui.ShowBanner(Version)
	}

	// Step 1: Load module configurations
	fmt.Println(i18n.T("info.loading_modules"))
	config, err := models.LoadModules()
	if err != nil {
		ui.ShowError(fmt.Errorf(i18n.T("err.load_modules"), err))
		os.Exit(1)
	}
	fmt.Println(i18n.Tf("info.modules_loaded", len(config.Modules)))
	fmt.Println()

	// Step 2: Module selection
	module, err := ui.ShowModuleMenu(config)
	if err != nil {
		ui.ShowError(err)
		os.Exit(1)
	}

	fmt.Println(i18n.Tf("info.module_selected", module.DisplayName))

	// Step 3: Binary manager and platform info
	binMgr := sys.NewBinaryManager()
	if err := binMgr.Initialize(); err != nil {
		ui.ShowError(fmt.Errorf(i18n.T("err.init_binaries"), err))
		os.Exit(1)
	}

	platformInfo := sys.DetectPlatform()

	// Override socket path from module's StationConfig ctrl_interface if present
	for _, p := range module.StationConfig {
		if p.Key == "ctrl_interface" && p.Value != "" {
			platformInfo.WpaSupplicantSocket = p.Value
			break
		}
	}

	platformInfo.Print()

	// Step 4: Main loop
	for {
		// Show options menu
		operation := ui.ShowOperationMenu()

		switch operation {
		case "1":
			if runDiagnosis(module) {
				continue
			}
		case "2":
			runConfigEditor(module, platformInfo)
			continue
		case "3":
			runStationMode(module, binMgr, platformInfo)
		case "4":
			runAPConfigEditor(module)
			continue
		case "5":
			runAPMode(module, binMgr)
			continue
		case "0":
			fmt.Println(i18n.T("prompt.goodbye"))
			return
		default:
			ui.ShowError(fmt.Errorf(i18n.T("menu.op.invalid")))
		}

		if !ui.PromptContinue() {
			fmt.Println(i18n.T("prompt.goodbye"))
			break
		}
	}
}

// runDiagnosis performs diagnostic checks, returns true if all checks passed
func runDiagnosis(module *models.WiFiModule) bool {
	fmt.Println(i18n.T("info.run_diagnosis"))
	result := wifi.RunDiagnosis(module)
	result.Print()

	// If driver is not loaded, offer to load it
	if !result.DriverLoaded {
		fmt.Print(i18n.T("prompt.load_driver"))
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			if err := wifi.LoadDriver(module); err != nil {
				ui.ShowError(fmt.Errorf(i18n.T("err.driver_load_fail"), err))
			} else {
				// Re-run diagnosis after loading driver
				fmt.Println(i18n.T("info.rerun_diagnosis"))
				result = wifi.RunDiagnosis(module)
				result.Print()
			}
		}
	}

	// All checks passed
	if len(result.Errors) == 0 {
		for {
			fmt.Println("\n" + ui.Yellow + ui.Bold + i18n.T("diag.post.title") + ui.Reset)
			fmt.Println(ui.Cyan + ui.Bold + "[1]" + ui.Reset + " " + i18n.T("diag.post.1"))
			fmt.Println(ui.Cyan + ui.Bold + "[2]" + ui.Reset + " " + i18n.T("diag.post.2"))
			fmt.Println(ui.Cyan + ui.Bold + "[0]" + ui.Reset + " " + i18n.T("diag.post.0"))
			fmt.Print("\n" + ui.Yellow + ui.Bold + i18n.T("diag.post.select") + ui.Reset)

			var choice string
			fmt.Scanln(&choice)

			switch choice {
			case "1":
				if err := wifi.UnloadDriver(module); err != nil {
					ui.ShowError(fmt.Errorf(i18n.T("err.unload_driver"), err))
				} else {
					fmt.Println(i18n.T("info.driver_unloaded"))
				}
				return true
			case "2":
				if err := wifi.UnloadDriver(module); err != nil {
					ui.ShowError(fmt.Errorf(i18n.T("err.unload_driver"), err))
				} else {
					if err := wifi.LoadDriver(module); err != nil {
						ui.ShowError(fmt.Errorf(i18n.T("err.reload_driver"), err))
					} else {
						fmt.Println(i18n.T("info.driver_reloaded"))
					}
				}
				return true
			case "0":
				return true
			default:
				fmt.Println(i18n.T("diag.post.invalid"))
			}
		}
	}

	return false
}

// runStationMode runs Station mode workflow
func runStationMode(module *models.WiFiModule, binMgr *sys.BinaryManager, platformInfo *sys.PlatformInfo) {
	supplicantPath := binMgr.GetWpaSupplicantPath()
	if module.ID == "quectel_fgh200m" {
		supplicantPath = "wpa_supplicant_s1g"
	}

	connector := wifi.NewConnector(
		module,
		supplicantPath,
		platformInfo.WpaCliPath,
		platformInfo.WpaSupplicantSocket, // already overridden from StationConfig
	)

	keepRunning, err := connector.Start()
	if err != nil {
		ui.ShowError(err)
		return
	}

	// If user chose to keep existing connection, don't stop the daemon
	if keepRunning {
		fmt.Println(i18n.T("info.wpa_running"))
		return
	}

	// For new connections, ask if user wants to disconnect
	fmt.Print(i18n.T("prompt.disconnect"))
	var response string
	fmt.Scanln(&response)

	if response == "y" || response == "Y" {
		fmt.Println(i18n.T("info.disconnecting"))
		connector.Stop()
	} else {
		fmt.Println(i18n.T("info.keep_connection"))
	}
}

// runConfigEditor runs the wpa_supplicant.conf editor
func runConfigEditor(module *models.WiFiModule, platformInfo *sys.PlatformInfo) {
	editor := config.NewWpaEditor(module, platformInfo)
	if _, err := editor.Start(); err != nil {
		ui.ShowError(err)
	}
}

// runAPConfigEditor runs the AP config generator/editor
func runAPConfigEditor(module *models.WiFiModule) {
	editor, err := config.NewAPConfigEditor(module)
	if err != nil {
		ui.ShowError(err)
		return
	}
	if err := editor.Start(); err != nil {
		ui.ShowError(err)
		return
	}
}

// runAPMode runs AP mode workflow
func runAPMode(module *models.WiFiModule, binMgr *sys.BinaryManager) {
	hostapdPath := binMgr.GetHostapdPath()
	if module.ID == "quectel_fgh200m" {
		hostapdPath = "hostapd_s1g"
	}

	hotspot := wifi.NewHotspot(
		module,
		hostapdPath,
	)

	if err := hotspot.Start(); err != nil {
		ui.ShowError(err)
		return
	}
}

