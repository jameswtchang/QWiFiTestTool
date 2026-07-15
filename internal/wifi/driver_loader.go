package wifi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
)

// LoadDriver attempts to load a kernel module using a three-tier search strategy:
// 1. Tool's directory (highest priority)
// 2. System common paths
// 3. Recursive search in /system/lib/modules/ and /lib/modules/
func LoadDriver(module *models.WiFiModule) error {
	// Enable rfkill and prepare PCIe bus for PCIe/CNSS-based drivers
	if strings.Contains(module.Kernel.PreloadKo, "pcie") || strings.Contains(module.Kernel.PreloadKo, "cnss") {
		fmt.Println("[INFO] 啟用無線電源...")
		exec.Command("sh", "-c", "echo 1 > /sys/class/rfkill/rfkill*/state 2>/dev/null").Run()
	}

	// Load prerequisite ko files first (e.g., BSP driver before functional driver)
	preloads := module.Kernel.GetPreloadKoList()
	for _, preKo := range preloads {
		fmt.Printf("[INFO] 載入前置驅動: %s\n", preKo)
		if err := findAndInsmod(preKo, ""); err != nil {
			// pcie-dw-rockchip may not exist on non-Rockchip boards; skip gracefully
			if strings.Contains(preKo, "pcie-dw-") {
				fmt.Printf("[WARN] 系統無 %s 或載入失敗，略過\n", preKo)
				continue
			}
			return fmt.Errorf("前置驅動載入失敗 %s: %w", preKo, err)
		}
		// After loading PCIe host controller, wait for link training then trigger bus rescan
		if strings.Contains(preKo, "pcie-dw-") {
			fmt.Println("[INFO] 等候 PCIe Link Training (3 秒)...")
			time.Sleep(3 * time.Second)
			exec.Command("sh", "-c", "echo 1 > /sys/bus/pci/rescan 2>/dev/null").Run()
			time.Sleep(1 * time.Second)
		}
	}

	// Load main driver
	koName := module.Kernel.KoName
	fmt.Printf("[INFO] 開始載入驅動: %s\n", koName)
	if err := findAndInsmod(koName, module.Kernel.InsmodArgs); err != nil {
		return err
	}

	// Wait for network interface to appear — only for PCIe/CNSS based drivers (Qualcomm QMI/MHI
	// firmware download takes ~4-6s). USB/SDIO drivers (Realtek, MTK, AIC) appear almost instantly.
	if strings.Contains(module.Kernel.PreloadKo, "pcie") || strings.Contains(module.Kernel.PreloadKo, "cnss") {
		ifName := module.Kernel.Interface
		if ifName == "" {
			ifName = "wlan0"
		}
		fmt.Printf("[INFO] 等候網路介面 %s 初始化 (最長 8 秒)...\n", ifName)
		for i := 0; i < 16; i++ {
			if _, err := os.Stat("/sys/class/net/" + ifName); err == nil {
				fmt.Printf("[SUCCESS] 網路介面 %s 已就緒\n", ifName)
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Execute post-load command if defined (e.g., "ifconfig wlan0 up" for Qualcomm)
	if module.Kernel.PostLoadCmd != "" {
		fmt.Printf("[INFO] 執行驅動後置命令: %s\n", module.Kernel.PostLoadCmd)
		cmd := exec.Command("sh", "-c", module.Kernel.PostLoadCmd)
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[WARN] 後置命令失敗: %s - %v\n", string(output), err)
		} else {
			fmt.Printf("[SUCCESS] 後置命令完成\n")
		}
	}

	return nil
}

// findAndInsmod searches for a .ko file using the three-tier strategy and loads it.
func findAndInsmod(koName, args string) error {
	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("無法獲取執行檔路徑: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Tier 1: Check tool's directory and its parent
	for _, dir := range []string{exeDir, filepath.Dir(exeDir)} {
		candidate := filepath.Join(dir, koName)
		if fileExists(candidate) {
			fmt.Printf("[INFO] 找到本地驅動: %s\n", candidate)
			return insmodDriver(candidate, args)
		}
	}

	// Tier 2: System common paths
	commonPaths := []string{
		fmt.Sprintf("/lib/modules/%s/kernel/drivers/net/wireless", getKernelVersion()),
		"/lib/modules/extra",
		"/usr/lib/modules",
	}

	for _, basePath := range commonPaths {
		candidatePath := filepath.Join(basePath, koName)
		if fileExists(candidatePath) {
			fmt.Printf("[INFO] 找到系統驅動: %s\n", candidatePath)
			return insmodDriver(candidatePath, args)
		}
	}

	// Tier 3: Recursive search in module directories
	fmt.Println("[INFO] 在模組目錄中搜尋驅動...")
	searchDirs := []string{
		"/system/lib/modules",
		"/lib/modules",
	}

	for _, dir := range searchDirs {
		if !dirExists(dir) {
			continue
		}

		foundPath, err := findKernelModule(dir, koName)
		if err == nil && foundPath != "" {
			fmt.Printf("[INFO] 找到驅動: %s\n", foundPath)
			return insmodDriver(foundPath, args)
		}
	}

	return fmt.Errorf("找不到驅動檔案: %s", koName)
}

// UnloadDriver removes a loaded kernel module
func UnloadDriver(module *models.WiFiModule) error {
	moduleName := strings.TrimSuffix(module.Kernel.KoName, ".ko")

	fmt.Printf("[INFO] 卸載驅動: %s\n", moduleName)

	rmmodPath := sys.FindCommand("rmmod", "rmmod")
	cmd := exec.Command(rmmodPath, moduleName)
	fmt.Printf("[DEBUG] 執行命令: %s %s\n", rmmodPath, moduleName)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("卸載失敗: %s - %w", string(output), err)
	}

	fmt.Printf("[SUCCESS] 已卸載驅動: %s\n", moduleName)

	// Unload prerequisite ko files in reverse order
	preloads := module.Kernel.GetPreloadKoList()
	for i := len(preloads) - 1; i >= 0; i-- {
		preModName := strings.TrimSuffix(filepath.Base(preloads[i]), ".ko")
		// Never unload PCIe controller or CNSS core — tearing down the bus hangs the kernel
		if strings.Contains(preModName, "pcie") || strings.Contains(preModName, "cnss") {
			fmt.Printf("[INFO] 保留底層匯流排驅動: %s\n", preModName)
			continue
		}
		fmt.Printf("[INFO] 卸載前置驅動: %s\n", preModName)
		cmd := exec.Command(rmmodPath, preModName)
		fmt.Printf("[DEBUG] 執行命令: %s %s\n", rmmodPath, preModName)
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[WARN] 前置驅動卸載失敗 %s: %s\n", preModName, string(output))
		} else {
			fmt.Printf("[SUCCESS] 已卸載前置驅動: %s\n", preModName)
		}
	}

	return nil
}

// insmodDriver loads a kernel module with optional arguments
func insmodDriver(koPath, args string) error {
	insmodPath := sys.FindCommand("insmod", "insmod")

	var cmd *exec.Cmd
	var cmdStr string

	if args != "" {
		argList := strings.Fields(args)
		execArgs := append([]string{koPath}, argList...)
		cmd = exec.Command(insmodPath, execArgs...)
		cmdStr = fmt.Sprintf("%s %s %s", insmodPath, koPath, args)
	} else {
		cmd = exec.Command(insmodPath, koPath)
		cmdStr = fmt.Sprintf("%s %s", insmodPath, koPath)
	}

	fmt.Printf("[DEBUG] 執行命令: %s\n", cmdStr)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Already loaded is not an error
		if strings.Contains(string(output), "File exists") {
			fmt.Printf("[INFO] 驅動已載入: %s\n", koPath)
			return nil
		}
		return fmt.Errorf("insmod 失敗: %s - %w", string(output), err)
	}

	fmt.Printf("[SUCCESS] 已載入驅動: %s\n", koPath)
	return nil
}

// findKernelModule recursively searches for a kernel module
func findKernelModule(baseDir, koName string) (string, error) {
	var foundPath string

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't access
		}

		if !info.IsDir() && info.Name() == koName {
			foundPath = path
			return filepath.SkipAll // Stop walking once found
		}

		return nil
	})

	if foundPath != "" {
		return foundPath, nil
	}

	if err != nil {
		return "", err
	}

	return "", fmt.Errorf("模組未找到")
}

// getKernelVersion returns the current kernel version
func getKernelVersion() string {
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return "$(uname -r)" // Fallback
	}
	return strings.TrimSpace(string(output))
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
