package wifi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
)

// LoadDriver attempts to load a kernel module using a three-tier search strategy:
// 1. Tool's directory (highest priority)
// 2. System common paths
// 3. Recursive search in /system/lib/modules/ and /lib/modules/
func LoadDriver(module *models.WiFiModule) error {
	// Enable rfkill and prepare PCIe bus for PCIe/CNSS-based drivers
	if strings.Contains(module.Kernel.PreloadKo, "pcie") || strings.Contains(module.Kernel.PreloadKo, "cnss") {
		fmt.Println(i18n.T("drv.enable_rf"))
		exec.Command("sh", "-c", "echo 1 > /sys/class/rfkill/rfkill*/state 2>/dev/null").Run()
	}

	// Load prerequisite ko files first (e.g., BSP driver before functional driver)
	preloads := module.Kernel.GetPreloadKoList()
	preloadArgs := module.Kernel.GetPreloadArgsList()
	for i, preKo := range preloads {
		args := ""
		if i < len(preloadArgs) {
			args = preloadArgs[i]
		}
		fmt.Println(i18n.Tf("drv.load_preload", preKo))
		if err := findAndInsmod(preKo, args); err != nil {
			// pcie-dw-rockchip may not exist on non-Rockchip boards; skip gracefully
			if strings.Contains(preKo, "pcie-dw-") {
				fmt.Println(i18n.Tf("drv.preload_skip", preKo))
				continue
			}
			return fmt.Errorf(i18n.T("drv.preload_fail"), preKo, err)
		}
		// After loading PCIe host controller, wait for link training then trigger bus rescan
		if strings.Contains(preKo, "pcie-dw-") {
			fmt.Println(i18n.T("drv.pcie_wait"))
			time.Sleep(3 * time.Second)
			exec.Command("sh", "-c", "echo 1 > /sys/bus/pci/rescan 2>/dev/null").Run()
			time.Sleep(1 * time.Second)
		}
	}

	// Load main driver
	koName := module.Kernel.KoName
	fmt.Println(i18n.Tf("drv.load_main", koName))
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
		fmt.Println(i18n.Tf("drv.wait_iface", ifName))
		for i := 0; i < 16; i++ {
			if _, err := os.Stat("/sys/class/net/" + ifName); err == nil {
				fmt.Println(i18n.Tf("drv.iface_ready", ifName))
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Execute post-load command if defined (e.g., "ifconfig wlan0 up" for Qualcomm)
	if module.Kernel.PostLoadCmd != "" {
		fmt.Println(i18n.Tf("drv.post_cmd", module.Kernel.PostLoadCmd))
		cmd := exec.Command("sh", "-c", module.Kernel.PostLoadCmd)
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Println(i18n.Tf("drv.post_cmd_fail", string(output), err))
		} else {
			fmt.Println(i18n.T("drv.post_cmd_ok"))
		}
	}

	return nil
}

// findAndInsmod searches for a .ko file using the three-tier strategy and loads it.
func findAndInsmod(koName, args string) error {
	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf(i18n.T("drv.exe_path_err"), err)
	}
	exeDir := filepath.Dir(exePath)

	// Tier 1: Check tool's directory and its parent
	for _, dir := range []string{exeDir, filepath.Dir(exeDir)} {
		candidate := filepath.Join(dir, koName)
		if fileExists(candidate) {
			fmt.Println(i18n.Tf("drv.found_local", candidate))
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
			fmt.Println(i18n.Tf("drv.found_sys", candidatePath))
			return insmodDriver(candidatePath, args)
		}
	}

	// Tier 3: Recursive search in module directories
	fmt.Println(i18n.T("drv.search_modules"))
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
			fmt.Println(i18n.Tf("drv.found", foundPath))
			return insmodDriver(foundPath, args)
		}
	}

	return fmt.Errorf(i18n.T("drv.not_found"), koName)
}

// UnloadDriver removes a loaded kernel module
func UnloadDriver(module *models.WiFiModule) error {
	iface := module.Kernel.GetInterface()

	// 1. Terminate user-space processes holding the network interface:
	//    wpa_supplicant/hostapd bound to it (or with unknown binding), DHCP clients/dnsmasq bound to it.
	//    Daemons serving other interfaces (e.g. eth0's DHCP lease) are kept.
	sys.KillDaemonsForIface([]string{"wpa_supplicant", "wpa_supplicant_s1g", "hostapd", "hostapd_s1g"}, iface)
	sys.KillProcsForIface([]string{"udhcpc", "dhclient", "dnsmasq"}, iface)

	// 2. Bring down the interface and flush addresses/routes so netdevice refcount drops
	if iface != "" {
		exec.Command("ip", "addr", "flush", "dev", iface).Run()
		exec.Command("ip", "route", "flush", "dev", iface).Run()
		exec.Command("ip", "link", "set", iface, "down").Run()
	}
	time.Sleep(300 * time.Millisecond)

	moduleName := strings.TrimSuffix(filepath.Base(module.Kernel.KoName), ".ko")

	fmt.Println(i18n.Tf("drv.unloading", moduleName))

	rmmodPath := sys.FindCommand("rmmod", "rmmod")
	cmd := exec.Command(rmmodPath, moduleName)
	fmt.Println(i18n.Tf("drv.exec_cmd", rmmodPath+" "+moduleName))
	output, err := cmd.CombinedOutput()

	if err != nil {
		outStr := string(output)
		// If module was not currently loaded, continue to preloads instead of aborting
		if strings.Contains(outStr, "not currently loaded") || strings.Contains(outStr, "No such file") {
			fmt.Println(i18n.Tf("drv.not_loaded", moduleName))
		} else {
			return fmt.Errorf(i18n.T("drv.unload_fail"), outStr, err)
		}
	} else {
		fmt.Println(i18n.Tf("drv.unloaded", moduleName))
	}

	// Unload prerequisite ko files in reverse order
	preloads := module.Kernel.GetPreloadKoList()
	for i := len(preloads) - 1; i >= 0; i-- {
		preModName := strings.TrimSuffix(filepath.Base(preloads[i]), ".ko")
		// Never unload PCIe controller or CNSS core — tearing down the bus hangs the kernel
		if strings.Contains(preModName, "pcie") || strings.Contains(preModName, "cnss") {
			fmt.Println(i18n.Tf("drv.keep_bus", preModName))
			continue
		}
		fmt.Println(i18n.Tf("drv.unload_preload", preModName))
		cmd := exec.Command(rmmodPath, preModName)
		fmt.Println(i18n.Tf("drv.exec_cmd", rmmodPath+" "+preModName))
		if output, err := cmd.CombinedOutput(); err != nil {
			outStr := string(output)
			if strings.Contains(outStr, "not currently loaded") || strings.Contains(outStr, "No such file") {
				continue
			}
			fmt.Println(i18n.Tf("drv.preload_unload_fail", preModName, outStr))
		} else {
			fmt.Println(i18n.Tf("drv.preload_unloaded", preModName))
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

	fmt.Println(i18n.Tf("drv.exec_cmd", cmdStr))

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Already loaded is not an error, but warn if custom arguments were specified
		if strings.Contains(string(output), "File exists") {
			if args != "" {
				fmt.Println(ui.Yellow + i18n.Tf("drv.already_loaded_params_warn", koPath, args) + ui.Reset)
			} else {
				fmt.Println(i18n.Tf("drv.already_loaded", koPath))
			}
			return nil
		}
		return fmt.Errorf(i18n.T("drv.insmod_fail"), string(output), err)
	}

	fmt.Println(i18n.Tf("drv.loaded", koPath))
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

	return "", fmt.Errorf(i18n.T("drv.module_not_found"))
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
