package sys

import (
	"bytes"
	"os"
	"os/exec"
	"sync"
)

var (
	wpaCapCache = make(map[string]map[string]bool)
	wpaCapMu    sync.RWMutex
	// MockWpaCaps allows unit tests to inject capability values
	MockWpaCaps map[string]bool
)

// CheckWpaBinarySupport inspects the wpa_supplicant executable file to determine
// if a specific network block parameter (e.g. "disable_ht40", "disable_he", "disable_vht")
// was compiled into the binary (such as via CONFIG_HT_OVERRIDES).
//
// In hostap C code, network parameters are declared in config.c struct tables with string literals.
// If the binary cannot be read (e.g. host development environment, binary not yet deployed,
// or mocked test), it returns true so development flows are not unexpectedly blocked.
func CheckWpaBinarySupport(binPath string, param string) bool {
	if MockWpaCaps != nil {
		if val, exists := MockWpaCaps[param]; exists {
			return val
		}
	}

	if binPath == "" || param == "" {
		return true
	}

	wpaCapMu.RLock()
	if caps, ok := wpaCapCache[binPath]; ok {
		if val, exists := caps[param]; exists {
			wpaCapMu.RUnlock()
			return val
		}
	}
	wpaCapMu.RUnlock()

	resolvedPath := binPath
	if p, err := exec.LookPath(binPath); err == nil {
		resolvedPath = p
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		// If binary cannot be found/read on disk (e.g. Windows unit test), default to true
		return true
	}

	wpaCapMu.Lock()
	defer wpaCapMu.Unlock()

	caps, ok := wpaCapCache[binPath]
	if !ok {
		caps = make(map[string]bool)
		wpaCapCache[binPath] = caps
	}

	if val, exists := caps[param]; exists {
		return val
	}

	// Share cache with resolvedPath if different
	if binPath != resolvedPath {
		wpaCapCache[resolvedPath] = caps
	}

	// Pre-populate common override parameters to save repetitive checks
	commonParams := []string{"disable_ht40", "disable_ht", "disable_vht", "disable_he", "disable_eht"}
	for _, p := range commonParams {
		if _, exists := caps[p]; !exists {
			caps[p] = bytes.Contains(data, []byte(p))
		}
	}

	if _, exists := caps[param]; !exists {
		caps[param] = bytes.Contains(data, []byte(param))
	}

	return caps[param]
}
