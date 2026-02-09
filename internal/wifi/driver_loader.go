package wifi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
)

// LoadDriver attempts to load a kernel module using a three-tier search strategy:
// 1. Tool's directory (highest priority)
// 2. System common paths
// 3. Recursive search in /system/lib/modules/ and /lib/modules/
func LoadDriver(module *models.WiFiModule) error {
	koName := module.Kernel.KoName
	insmodArgs := module.Kernel.InsmodArgs

	fmt.Printf("[INFO] 開始載入驅動: %s\n", koName)

	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("無法獲取執行檔路徑: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Tier 1: Check tool's directory
	localPath := filepath.Join(exeDir, koName)
	if fileExists(localPath) {
		fmt.Printf("[INFO] 找到本地驅動: %s\n", localPath)
		return insmodDriver(localPath, insmodArgs)
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
			return insmodDriver(candidatePath, insmodArgs)
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
			return insmodDriver(foundPath, insmodArgs)
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
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("卸載失敗: %s - %w", string(output), err)
	}

	fmt.Printf("[SUCCESS] 已卸載驅動: %s\n", moduleName)
	return nil
}

// insmodDriver loads a kernel module with optional arguments
func insmodDriver(koPath, args string) error {
	insmodPath := sys.FindCommand("insmod", "insmod")

	var cmd *exec.Cmd
	var cmdStr string

	if args != "" {
		cmd = exec.Command(insmodPath, koPath, args)
		cmdStr = fmt.Sprintf("%s %s %s", insmodPath, koPath, args)
	} else {
		cmd = exec.Command(insmodPath, koPath)
		cmdStr = fmt.Sprintf("%s %s", insmodPath, koPath)
	}

	fmt.Printf("[CMD] %s\n", cmdStr)

	output, err := cmd.CombinedOutput()
	if err != nil {
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
