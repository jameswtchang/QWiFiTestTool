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
	fmt.Println(Cyan + Bold + "╔═══════════════════════════════════════════╗" + Reset)
	fmt.Println(Cyan + Bold + "║     QWifiTestTool - WiFi 配置與測試工具   ║" + Reset)
	fmt.Println(Cyan + Bold + "║           支援 Linux & Android            ║" + Reset)
	fmt.Println(Cyan + Bold + "║                版本: V1.0                 ║" + Reset)
	fmt.Println(Cyan + Bold + "║          Designed by James Chang          ║" + Reset)
	fmt.Println(Cyan + Bold + "╚═══════════════════════════════════════════╝" + Reset)
	fmt.Println()
}

// ShowModuleMenu displays available modules and prompts for selection
func ShowModuleMenu(config *models.WiFiToolConfig) (*models.WiFiModule, error) {
	fmt.Println(Blue + Bold + "========== 可用的 WiFi 模組 ==========" + Reset)

	modules := config.ListModules()
	for _, mod := range modules {
		modStr := strings.Replace(mod, "[", Cyan+Bold+"[", 1)
		modStr = strings.Replace(modStr, "]", "]"+Reset, 1)
		fmt.Println(modStr)
	}

	fmt.Println(Blue + Bold + "=====================================" + Reset)
	fmt.Print(Yellow + Bold + "選擇模組編號: " + Reset)

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
	fmt.Println("\n" + Yellow + Bold + "========== 操作模式 ==========" + Reset)
	fmt.Println(Cyan + Bold + "[1]" + Reset + " 診斷 (Sanity Check)")
	fmt.Println(Cyan + Bold + "[2]" + Reset + " 編輯 wpa_supplicant.conf")
	fmt.Println(Cyan + Bold + "[3]" + Reset + " Station 模式 (連接 WiFi)")
	fmt.Println(Cyan + Bold + "[4]" + Reset + " Hostapd/Dnsmasq 配置生成及修改")
	fmt.Println(Cyan + Bold + "[5]" + Reset + " AP 模式 (建立熱點)")
	fmt.Println(Cyan + Bold + "[0]" + Reset + " 退出")
	fmt.Println(Yellow + Bold + "============================" + Reset)
	fmt.Print(Yellow + Bold + "選擇操作: " + Reset)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// PromptContinue asks user if they want to continue
func PromptContinue() bool {
	fmt.Print("\n" + Yellow + Bold + "繼續其他操作? [y/N]: " + Reset)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.ToLower(strings.TrimSpace(input))

	return input == "y" || input == "yes"
}

// ShowError displays an error message
func ShowError(err error) {
	fmt.Printf("\n"+Red+Bold+"❌ 錯誤: %v"+Reset+"\n\n", err)
}

// ShowSuccess displays a success message
func ShowSuccess(msg string) {
	fmt.Printf("\n"+Green+Bold+"✓ %s"+Reset+"\n\n", msg)
}
