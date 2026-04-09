package main

import (
	"fmt"
	"os"

	"qwifitesttool/internal/config"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
	"qwifitesttool/internal/wifi"
)

func main() {
	// Show banner
	ui.ShowBanner()

	// Step 1: Load module configurations
	fmt.Println("[INFO] 載入模組配置...")
	config, err := models.LoadModules()
	if err != nil {
		ui.ShowError(fmt.Errorf("failed to load modules: %w", err))
		os.Exit(1)
	}
	fmt.Printf("[INFO] 已載入 %d 個模組\n\n", len(config.Modules))

	// Step 2: Module selection
	module, err := ui.ShowModuleMenu(config)
	if err != nil {
		ui.ShowError(err)
		os.Exit(1)
	}

	fmt.Printf("\n[INFO] 已選擇模組: %s\n", module.DisplayName)

	// Step 3: Binary manager and platform info
	binMgr := sys.NewBinaryManager()
	if err := binMgr.Initialize(); err != nil {
		ui.ShowError(fmt.Errorf("failed to initialize binaries: %w", err))
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
			runDiagnosis(module)
		case "2":
			runConfigEditor(module, platformInfo)
			continue
		case "3":
			runStationMode(module, binMgr, platformInfo)
		case "4":
			runAPConfigEditor(module)
		case "5":
			runAPMode(module, binMgr)
		case "0":
			fmt.Println("再見!")
			return
		default:
			ui.ShowError(fmt.Errorf("無效的選項"))
		}

		if !ui.PromptContinue() {
			fmt.Println("再見!")
			break
		}
	}
}

// runDiagnosis performs diagnostic checks
func runDiagnosis(module *models.WiFiModule) {
	fmt.Println("\n[INFO] 執行診斷檢查...")
	result := wifi.RunDiagnosis(module)
	result.Print()

	// If driver is not loaded, offer to load it
	if !result.DriverLoaded {
		fmt.Print("\n驅動未載入。是否嘗試載入驅動? [y/N]: ")
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			if err := wifi.LoadDriver(module); err != nil {
				ui.ShowError(fmt.Errorf("驅動載入失敗: %w", err))
			} else {
				// Re-run diagnosis after loading driver
				fmt.Println("\n[INFO] 重新執行診斷...")
				result := wifi.RunDiagnosis(module)
				result.Print()
			}
		}
	}
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
		fmt.Println("\n[INFO] wpa_supplicant 持續運行中，連線已保持")
		return
	}

	// For new connections, ask if user wants to disconnect
	fmt.Print("\n是否斷開連線並停止 wpa_supplicant? [y/N]: ")
	var response string
	fmt.Scanln(&response)

	if response == "y" || response == "Y" {
		fmt.Println("[INFO] 斷開連線...")
		connector.Stop()
	} else {
		fmt.Println("[INFO] 保持連線，wpa_supplicant 持續運行")
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
