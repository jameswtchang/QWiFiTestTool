package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"qwifitesttool/internal/config"
	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
	"qwifitesttool/internal/wifi"
)

// Version defines the application version string
const Version = "V1.18"

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

	// Android framework WiFi takeover: disabled lazily right before a daemon starts,
	// restored on exit unless a Station connection is intentionally kept alive.
	svc := sys.NewServiceManager(platformInfo.IsAndroid)
	keepStation := false

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
			keepStation = runStationMode(module, binMgr, platformInfo, svc)
		case "4":
			runAPConfigEditor(module)
			continue
		case "5":
			runAPMode(module, binMgr, svc)
			keepStation = false // AP mode kills wpa_supplicant on start
			continue
		case "0":
			restoreAndroidWiFi(svc, keepStation)
			fmt.Println(i18n.T("prompt.goodbye"))
			return
		default:
			ui.ShowError(fmt.Errorf(i18n.T("menu.op.invalid")))
		}

		if !ui.PromptContinue() {
			restoreAndroidWiFi(svc, keepStation)
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

	// If driver is not loaded or parameters mismatch, offer to load/reload it
	if !result.DriverLoaded || result.ParamMismatch {
		promptKey := "prompt.load_driver"
		if result.DriverLoaded && result.ParamMismatch {
			promptKey = "prompt.reload_driver_mismatch"
		}
		fmt.Print(i18n.T(promptKey))
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			if result.ParamMismatch {
				// Must unload first so new parameters can be passed to insmod
				_ = wifi.UnloadDriver(module)
			}
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

// restoreAndroidWiFi re-enables the Android WiFi framework on exit.
// Skipped while a Station connection is kept, since the framework would kill our wpa_supplicant.
func restoreAndroidWiFi(svc *sys.ServiceManager, keepStation bool) {
	if !svc.IsDisabled() {
		return
	}
	if keepStation {
		fmt.Println(i18n.T("info.android_wifi_kept_off"))
		return
	}
	if err := svc.EnableAndroidWiFi(); err != nil {
		fmt.Printf("[WARN] %v\n", err)
	}
}

// runStationMode runs Station mode workflow, returns true if wpa_supplicant is left running
func runStationMode(module *models.WiFiModule, binMgr *sys.BinaryManager, platformInfo *sys.PlatformInfo, svc *sys.ServiceManager) bool {
	supplicantPath := binMgr.GetWpaSupplicantPathFor(module)

	connector := wifi.NewConnector(
		module,
		supplicantPath,
		platformInfo.WpaCliPath,
		platformInfo.WpaSupplicantSocket, // already overridden from StationConfig
		svc,
	)

	// Intercept Ctrl+C / SIGTERM during Station mode to guarantee daemon and interface cleanup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	doneChan := make(chan struct{})
	defer close(doneChan)

	go func() {
		select {
		case <-sigChan:
			fmt.Println("\n" + ui.Yellow + i18n.T("info.station_sigint") + ui.Reset)
			connector.Stop()
			restoreAndroidWiFi(svc, false)
			os.Exit(130)
		case <-doneChan:
			return
		}
	}()

	keepRunning, err := connector.Start()
	if err != nil {
		ui.ShowError(err)
		return false
	}

	// If user chose to keep existing connection, don't stop the daemon
	if keepRunning {
		fmt.Println(i18n.T("info.wpa_running"))
		return true
	}

	// For new connections, ask if user wants to disconnect
	fmt.Print(i18n.T("prompt.disconnect"))
	var response string
	fmt.Scanln(&response)

	if response == "y" || response == "Y" {
		fmt.Println(i18n.T("info.disconnecting"))
		connector.Stop()
		return false
	}
	fmt.Println(i18n.T("info.keep_connection"))
	return true
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
func runAPMode(module *models.WiFiModule, binMgr *sys.BinaryManager, svc *sys.ServiceManager) {
	hostapdPath := binMgr.GetHostapdPathFor(module)

	hotspot := wifi.NewHotspot(
		module,
		hostapdPath,
		svc,
	)

	if err := hotspot.Start(); err != nil {
		ui.ShowError(err)
		return
	}
}

