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
}

// RunDiagnosis performs pre-flight sanity checks on the WiFi module
func RunDiagnosis(module *models.WiFiModule) *DiagnosisResult {
	result := &DiagnosisResult{
		KernelLogs: make([]string, 0),
		Errors:     make([]string, 0),
	}

	iface := module.Kernel.GetInterface()

	// Check 1: Interface existence
	result.checkInterface(iface)

	// Check 2: Driver status
	result.checkDriver(module.Kernel.KoName)

	// Check 3: RFKill status
	result.checkRFKill()

	// Check 4: Kernel logs
	result.fetchKernelLogs(module.Kernel.KoName)

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

// checkDriver verifies if the kernel module is loaded
func (dr *DiagnosisResult) checkDriver(koName string) {
	// Read /proc/modules
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		dr.DriverLoaded = false
		dr.DriverInfo = "unable to read /proc/modules"
		dr.Errors = append(dr.Errors, "Cannot access /proc/modules")
		return
	}

	// Extract module name (remove .ko extension)
	moduleName := strings.TrimSuffix(koName, ".ko")

	// Fuzzy match: check if module name appears in /proc/modules
	if strings.Contains(string(data), moduleName) {
		dr.DriverLoaded = true

		// Extract the matching line
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.Contains(line, moduleName) {
				dr.DriverInfo = strings.TrimSpace(line)
				break
			}
		}
	} else {
		dr.DriverLoaded = false
		dr.DriverInfo = fmt.Sprintf("Module %s not loaded", moduleName)
		dr.Errors = append(dr.Errors, fmt.Sprintf("Driver %s is not loaded", moduleName))
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

// fetchKernelLogs retrieves recent kernel logs related to the module
func (dr *DiagnosisResult) fetchKernelLogs(koName string) {
	moduleName := strings.TrimSuffix(koName, ".ko")

	// Use dynamic command lookup for dmesg
	dmesgPath := sys.FindCommand("dmesg", "dmesg")
	cmd := exec.Command(dmesgPath)
	output, err := cmd.Output()
	if err != nil {
		dr.KernelLogs = append(dr.KernelLogs, "Unable to fetch dmesg")
		return
	}

	lines := strings.Split(string(output), "\n")
	matchCount := 0

	// Get last 5 matching lines
	for i := len(lines) - 1; i >= 0 && matchCount < 5; i-- {
		line := lines[i]
		if strings.Contains(strings.ToLower(line), strings.ToLower(moduleName)) {
			dr.KernelLogs = append([]string{strings.TrimSpace(line)}, dr.KernelLogs...)
			matchCount++
		}
	}

	if matchCount == 0 {
		dr.KernelLogs = append(dr.KernelLogs, fmt.Sprintf("No kernel logs found for %s", moduleName))
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
	fmt.Println("\n========== 診斷結果 ==========")

	// Interface
	fmt.Printf("介面存在: %v\n", dr.InterfaceExists)
	fmt.Printf("介面狀態: %s\n", dr.InterfaceState)

	// Driver
	fmt.Printf("驅動已載入: %v\n", dr.DriverLoaded)
	fmt.Printf("驅動資訊: %s\n", dr.DriverInfo)

	// RFKill
	fmt.Printf("RFKill 封鎖: %v\n", dr.RFKillBlocked)
	fmt.Printf("RFKill 資訊: %s\n", dr.RFKillInfo)

	// Kernel logs
	fmt.Println("\n核心日誌 (最近 5 條):")
	for _, log := range dr.KernelLogs {
		fmt.Printf("  %s\n", log)
	}

	// Errors
	if len(dr.Errors) > 0 {
		fmt.Println("\n⚠️  錯誤:")
		for _, err := range dr.Errors {
			fmt.Printf("  - %s\n", err)
		}
	} else {
		fmt.Println("\n✓ 所有檢查通過")
	}

	fmt.Println("==============================")
}
