package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"qwifitesttool/internal/models"
)

// ShowBanner displays the application banner
func ShowBanner() {
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║     QWifiTestTool - WiFi 配置與測試工具   ║")
	fmt.Println("║           支援 Linux & Android            ║")
	fmt.Println("╚═══════════════════════════════════════════╝")
	fmt.Println()
}

// ShowModuleMenu displays available modules and prompts for selection
func ShowModuleMenu(config *models.WiFiToolConfig) (*models.WiFiModule, error) {
	fmt.Println("========== 可用的 WiFi 模組 ==========")

	modules := config.ListModules()
	for _, mod := range modules {
		fmt.Println(mod)
	}

	fmt.Println("=====================================")
	fmt.Print("選擇模組編號: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var selection int
	fmt.Sscanf(input, "%d", &selection)

	if selection < 1 || selection > len(config.Modules) {
		return nil, fmt.Errorf("invalid selection")
	}

	return &config.Modules[selection-1], nil
}

// ShowOperationMenu displays operation mode menu
func ShowOperationMenu() string {
	fmt.Println("\n========== 操作模式 ==========")
	fmt.Println("[1] 診斷 (Sanity Check)")
	fmt.Println("[2] 編輯 wpa_supplicant.conf")
	fmt.Println("[3] Station 模式 (連接 WiFi)")
	fmt.Println("[4] Hostapd/Dnsmasq 配置生成及修改")
	fmt.Println("[5] AP 模式 (建立熱點)")
	fmt.Println("[0] 退出")
	fmt.Println("============================")
	fmt.Print("選擇操作: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// PromptContinue asks user if they want to continue
func PromptContinue() bool {
	fmt.Print("\n繼續其他操作? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.ToLower(strings.TrimSpace(input))

	return input == "y" || input == "yes"
}

// ShowError displays an error message
func ShowError(err error) {
	fmt.Printf("\n❌ 錯誤: %v\n\n", err)
}

// ShowSuccess displays a success message
func ShowSuccess(msg string) {
	fmt.Printf("\n✓ %s\n\n", msg)
}
