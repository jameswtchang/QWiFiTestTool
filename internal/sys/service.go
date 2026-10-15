package sys

import (
	"fmt"
	"os/exec"
	"strings"
)

// ServiceManager handles Android WiFi service takeover
type ServiceManager struct {
	isAndroid bool
	disabled  bool
}

// NewServiceManager creates a new service manager instance
func NewServiceManager(isAndroid bool) *ServiceManager {
	return &ServiceManager{
		isAndroid: isAndroid,
		disabled:  false,
	}
}

// DisableAndroidWiFi disables Android framework WiFi services (Android 12-16)
// It is idempotent and nil-safe, and only marks the framework for restoration
// when WiFi was actually enabled before the takeover.
func (sm *ServiceManager) DisableAndroidWiFi() error {
	if sm == nil || !sm.isAndroid {
		return nil // Nothing to do on Linux
	}
	if sm.disabled {
		return nil // Already taken over in this session
	}

	// Android 11+: skip takeover if the user already turned WiFi off,
	// so EnableAndroidWiFi does not turn it on unexpectedly at exit.
	if out, err := exec.Command("cmd", "wifi", "status").CombinedOutput(); err == nil &&
		strings.Contains(string(out), "Wifi is disabled") {
		fmt.Println("[INFO] Android WiFi framework already disabled, no takeover needed")
		return nil
	}

	fmt.Println("[INFO] Attempting to disable Android WiFi framework...")

	// Strategy 1: Try Android 13+ command
	if err := sm.tryCommand("cmd", []string{"wifi", "set-wifi-enabled", "disabled"}); err == nil {
		fmt.Println("[INFO] WiFi disabled using 'cmd wifi' (Android 13+)")
		sm.disabled = true
		return sm.killWpaSupplicant()
	}

	// Strategy 2: Try Android 12 command
	if err := sm.tryCommand("svc", []string{"wifi", "disable"}); err == nil {
		fmt.Println("[INFO] WiFi disabled using 'svc wifi' (Android 12)")
		sm.disabled = true
		return sm.killWpaSupplicant()
	}

	// Strategy 3: Just kill wpa_supplicant processes
	fmt.Println("[WARN] Framework commands failed, attempting to kill wpa_supplicant directly")
	return sm.killWpaSupplicant()
}

// EnableAndroidWiFi re-enables Android framework WiFi services
func (sm *ServiceManager) EnableAndroidWiFi() error {
	if sm == nil || !sm.isAndroid || !sm.disabled {
		return nil
	}

	fmt.Println("[INFO] Re-enabling Android WiFi framework...")

	// Try Android 13+ command
	if err := sm.tryCommand("cmd", []string{"wifi", "set-wifi-enabled", "enabled"}); err == nil {
		fmt.Println("[INFO] WiFi re-enabled using 'cmd wifi'")
		sm.disabled = false
		return nil
	}

	// Try Android 12 command
	if err := sm.tryCommand("svc", []string{"wifi", "enable"}); err == nil {
		fmt.Println("[INFO] WiFi re-enabled using 'svc wifi'")
		sm.disabled = false
		return nil
	}

	return fmt.Errorf("failed to re-enable WiFi framework")
}

// killWpaSupplicant kills all running wpa_supplicant processes
func (sm *ServiceManager) killWpaSupplicant() error {
	// Use dynamic command lookup for pkill
	pkillPath := FindCommand("pkill", "pkill")
	cmd := exec.Command(pkillPath, "-9", "wpa_supplicant")
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		// pkill returns error if no processes found, which is acceptable
		if strings.Contains(string(output), "no process found") {
			fmt.Println("[INFO] No wpa_supplicant processes to kill")
			return nil
		}
		fmt.Printf("[WARN] pkill wpa_supplicant failed: %v\n", err)
		return nil // Non-fatal
	}

	fmt.Println("[INFO] Killed existing wpa_supplicant processes")
	return nil
}

// tryCommand executes a command and returns error if it fails
func (sm *ServiceManager) tryCommand(name string, args []string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		fmt.Printf("[DEBUG] Command '%s %v' failed: %v, output: %s\n", 
			name, args, err, string(output))
		return err
	}
	
	return nil
}

// IsDisabled returns true if WiFi framework has been disabled
func (sm *ServiceManager) IsDisabled() bool {
	return sm != nil && sm.disabled
}
