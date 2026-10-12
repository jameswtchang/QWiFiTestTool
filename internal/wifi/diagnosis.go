package wifi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"qwifitesttool/internal/i18n"
	"qwifitesttool/internal/models"
	"qwifitesttool/internal/sys"
	"qwifitesttool/internal/ui"
)

// DiagnosisResult holds the results of sanity checks
type DiagnosisResult struct {
	InterfaceExists bool
	InterfaceState  string
	DriverLoaded    bool
	DriverInfo      string
	RFKillBlocked   bool
	RFKillInfo      string
	KernelLogs      []string
	Errors          []string
	ParamMismatch   bool
	MismatchDetails []string
}

// RunDiagnosis performs pre-flight sanity checks on the WiFi module
func RunDiagnosis(module *models.WiFiModule) *DiagnosisResult {
	result := &DiagnosisResult{
		KernelLogs:      make([]string, 0),
		Errors:          make([]string, 0),
		MismatchDetails: make([]string, 0),
	}

	if module == nil {
		result.Errors = append(result.Errors, "WiFi module is not specified")
		return result
	}

	iface := module.Kernel.GetInterface()

	// Check 1: Interface existence
	result.checkInterface(iface)

	// Check 2: Driver status (checks both main driver and prerequisite PreloadKo modules)
	result.checkDrivers(module)

	// Check 3: RFKill status
	result.checkRFKill()

	// Check 4: Kernel logs (retrieves logs for main and prerequisite drivers)
	result.fetchKernelLogs(module)

	return result
}

// checkInterface verifies if the network interface exists
func (dr *DiagnosisResult) checkInterface(iface string) {
	sysPath := filepath.Join("/sys/class/net", iface)

	if _, err := os.Stat(sysPath); err == nil {
		dr.InterfaceExists = true

		// Try to read operstate
		statePath := filepath.Join(sysPath, "operstate")
		if data, err := os.ReadFile(statePath); err == nil {
			dr.InterfaceState = strings.TrimSpace(string(data))
		} else {
			dr.InterfaceState = "unknown"
		}
	} else {
		dr.InterfaceExists = false
		dr.InterfaceState = "not found"
		dr.Errors = append(dr.Errors, fmt.Sprintf("Interface %s does not exist", iface))
	}
}

// normalizeModuleName removes .ko suffix and converts hyphens to underscores for kernel module matching
func normalizeModuleName(koName string) string {
	name := strings.TrimSuffix(filepath.Base(koName), ".ko")
	return strings.ReplaceAll(name, "-", "_")
}

// checkDrivers verifies if the main driver and prerequisite preload drivers are loaded and checks parameters
func (dr *DiagnosisResult) checkDrivers(module *models.WiFiModule) {
	// Read /proc/modules
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		dr.DriverLoaded = false
		dr.DriverInfo = "unable to read /proc/modules"
		dr.Errors = append(dr.Errors, "Cannot access /proc/modules")
		return
	}

	loadedModules := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			loadedModules[fields[0]] = strings.TrimSpace(line)
		}
	}

	var statusList []string
	allPreloadsLoaded := true
	mainLoaded := false

	// 1. Check prerequisite drivers (PreloadKo)
	preloads := module.Kernel.GetPreloadKoList()
	preloadArgs := module.Kernel.GetPreloadArgsList()
	for i, preKo := range preloads {
		modName := normalizeModuleName(preKo)
		if _, ok := loadedModules[modName]; ok {
			statusList = append(statusList, fmt.Sprintf("[Preload] %s: loaded", modName))
			// Check parameters if specified
			if i < len(preloadArgs) && preloadArgs[i] != "" {
				dr.checkModuleParameters(modName, preloadArgs[i])
			}
		} else {
			// pcie-dw-rockchip may be built-in (=y) or non-Rockchip board, skip gracefully
			if strings.Contains(preKo, "pcie-dw-") {
				statusList = append(statusList, fmt.Sprintf("[Preload] %s: skipped (built-in/opt)", modName))
				continue
			}
			allPreloadsLoaded = false
			statusList = append(statusList, fmt.Sprintf("[Preload] %s: NOT loaded", modName))
			dr.Errors = append(dr.Errors, fmt.Sprintf("Preload driver %s is not loaded", preKo))
		}
	}

	// 2. Check main driver (KoName)
	if module.Kernel.KoName != "" {
		mainModName := normalizeModuleName(module.Kernel.KoName)
		if _, ok := loadedModules[mainModName]; ok {
			mainLoaded = true
			statusList = append(statusList, fmt.Sprintf("[Main] %s: loaded", mainModName))
			if module.Kernel.InsmodArgs != "" {
				dr.checkModuleParameters(mainModName, module.Kernel.InsmodArgs)
			}
		} else {
			mainLoaded = false
			statusList = append(statusList, fmt.Sprintf("[Main] %s: NOT loaded", mainModName))
			dr.Errors = append(dr.Errors, fmt.Sprintf("Main driver %s is not loaded", module.Kernel.KoName))
		}
	} else {
		mainLoaded = true
	}

	dr.DriverLoaded = allPreloadsLoaded && mainLoaded
	dr.DriverInfo = strings.Join(statusList, " | ")
}

// checkModuleParameters inspects /sys/module/<modName>/parameters/ to verify whether active parameters match expected args
func (dr *DiagnosisResult) checkModuleParameters(modName, argsStr string) {
	for _, field := range strings.Fields(argsStr) {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) == 2 {
			paramKey := strings.TrimSpace(parts[0])
			expectedVal := strings.TrimSpace(parts[1])
			paramPath := filepath.Join("/sys/module", modName, "parameters", paramKey)
			if data, err := os.ReadFile(paramPath); err == nil {
				actualVal := strings.TrimSpace(string(data))
				if actualVal != expectedVal {
					dr.ParamMismatch = true
					msg := fmt.Sprintf("Module %s parameter mismatch: %s=%s (expected %s)", modName, paramKey, actualVal, expectedVal)
					dr.MismatchDetails = append(dr.MismatchDetails, msg)
					dr.Errors = append(dr.Errors, msg)
				}
			}
		}
	}
}

// checkRFKill checks for hardware/software radio blocks
func (dr *DiagnosisResult) checkRFKill() {
	rfkillPath := "/sys/class/rfkill"

	entries, err := os.ReadDir(rfkillPath)
	if err != nil {
		dr.RFKillBlocked = false
		dr.RFKillInfo = "rfkill not available"
		return
	}

	blocked := false
	var info []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Read soft and hard block status
		softPath := filepath.Join(rfkillPath, entry.Name(), "soft")
		hardPath := filepath.Join(rfkillPath, entry.Name(), "hard")

		soft := readSingleByte(softPath)
		hard := readSingleByte(hardPath)

		if soft == '1' || hard == '1' {
			blocked = true
			info = append(info, fmt.Sprintf("%s: soft=%c hard=%c", entry.Name(), soft, hard))
		}
	}

	dr.RFKillBlocked = blocked
	if len(info) > 0 {
		dr.RFKillInfo = strings.Join(info, "; ")
		dr.Errors = append(dr.Errors, "RFKill is blocking WiFi")
	} else {
		dr.RFKillInfo = "no blocks detected"
	}
}

// fetchKernelLogs retrieves recent kernel logs related to the main module and preload modules
func (dr *DiagnosisResult) fetchKernelLogs(module *models.WiFiModule) {
	var keywords []string
	if module.Kernel.KoName != "" {
		keywords = append(keywords, strings.ToLower(normalizeModuleName(module.Kernel.KoName)))
	}
	for _, preKo := range module.Kernel.GetPreloadKoList() {
		norm := strings.ToLower(normalizeModuleName(preKo))
		if norm != "" && !strings.Contains(norm, "pcie_dw") {
			keywords = append(keywords, norm)
		}
	}

	dmesgPath := sys.FindCommand("dmesg", "dmesg")
	cmd := exec.Command(dmesgPath)
	output, err := cmd.Output()
	if err != nil {
		dr.KernelLogs = append(dr.KernelLogs, "Unable to fetch dmesg")
		return
	}

	lines := strings.Split(string(output), "\n")
	matchCount := 0

	// Get last 8 matching lines
	for i := len(lines) - 1; i >= 0 && matchCount < 8; i-- {
		line := lines[i]
		lower := strings.ToLower(line)
		for _, kw := range keywords {
			if kw != "" && strings.Contains(lower, kw) {
				dr.KernelLogs = append([]string{strings.TrimSpace(line)}, dr.KernelLogs...)
				matchCount++
				break
			}
		}
	}

	if matchCount == 0 {
		dr.KernelLogs = append(dr.KernelLogs, fmt.Sprintf("No kernel logs found for [%s]", strings.Join(keywords, ", ")))
	}
}

// readSingleByte reads a single byte from a file (for rfkill status)
func readSingleByte(path string) byte {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return '?'
	}
	return data[0]
}

// Print formats and displays the diagnosis results
func (dr *DiagnosisResult) Print() {
	fmt.Println("\n" + ui.Blue + ui.Bold + "========== " + i18n.T("diag.result.title") + " ==========" + ui.Reset)

	// Interface
	if dr.InterfaceExists {
		fmt.Printf(ui.Bold+i18n.T("diag.iface_exists")+ui.Green+"%v"+ui.Reset+"\n", dr.InterfaceExists)
	} else {
		fmt.Printf(ui.Bold+i18n.T("diag.iface_exists")+ui.Red+"%v"+ui.Reset+"\n", dr.InterfaceExists)
	}
	fmt.Printf(ui.Bold+i18n.T("diag.iface_state")+ui.Cyan+"%s"+ui.Reset+"\n", dr.InterfaceState)

	// Driver
	if dr.DriverLoaded && !dr.ParamMismatch {
		fmt.Printf(ui.Bold+i18n.T("diag.driver_loaded")+ui.Green+"%v"+ui.Reset+"\n", dr.DriverLoaded)
	} else {
		fmt.Printf(ui.Bold+i18n.T("diag.driver_loaded")+ui.Red+"%v"+ui.Reset+"\n", dr.DriverLoaded)
	}
	fmt.Printf(ui.Bold+i18n.T("diag.driver_info")+ui.Cyan+"%s"+ui.Reset+"\n", dr.DriverInfo)

	if dr.ParamMismatch {
		for _, detail := range dr.MismatchDetails {
			fmt.Println("  " + ui.Yellow + "[WARN] " + detail + ui.Reset)
		}
	}

	// RFKill
	if !dr.RFKillBlocked {
		fmt.Printf(ui.Bold+i18n.T("diag.rfkill_blocked")+ui.Green+"%v"+ui.Reset+"\n", dr.RFKillBlocked)
	} else {
		fmt.Printf(ui.Bold+i18n.T("diag.rfkill_blocked")+ui.Red+"%v"+ui.Reset+"\n", dr.RFKillBlocked)
	}
	fmt.Printf(ui.Bold+i18n.T("diag.rfkill_info")+ui.Cyan+"%s"+ui.Reset+"\n", dr.RFKillInfo)

	// Kernel logs
	fmt.Println("\n" + ui.Bold + i18n.T("diag.kernel_logs") + ui.Reset)
	for _, log := range dr.KernelLogs {
		fmt.Printf("  "+ui.White+"%s"+ui.Reset+"\n", log)
	}

	// Errors
	if len(dr.Errors) > 0 {
		fmt.Println("\n" + ui.Red + ui.Bold + i18n.T("diag.errors") + ui.Reset)
		for _, err := range dr.Errors {
			fmt.Printf("  "+ui.Red+"- %s"+ui.Reset+"\n", err)
		}
	} else {
		fmt.Println("\n" + ui.Green + ui.Bold + i18n.T("diag.all_pass") + ui.Reset)
	}

	fmt.Println(ui.Blue + ui.Bold + "==============================" + ui.Reset)
}
