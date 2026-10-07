package models

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

// WiFiToolConfig represents the root XML structure
type WiFiToolConfig struct {
	XMLName xml.Name     `xml:"WiFiTool"`
	Modules []WiFiModule `xml:"Module"`
}

// WiFiModule represents a single WiFi module configuration
type WiFiModule struct {
	ID            string       `xml:"id,attr"`
	DisplayName   string       `xml:"DisplayName"`
	Kernel        KernelConfig `xml:"Kernel"`
	StationConfig []Param      `xml:"StationConfig>Param"`
	APConfig      APConfig     `xml:"APConfig"`
}

// KernelConfig contains kernel-level module parameters
type KernelConfig struct {
	KoName        string `xml:"KoName"`        // Kernel module filename (e.g., 8821cu.ko)
	PreloadKo         string `xml:"PreloadKo"`         // Prerequisite .ko files to load before KoName (comma-separated)
	PreloadInsmodArgs string `xml:"PreloadInsmodArgs"` // Optional arguments for prerequisite .ko files (comma-separated matching PreloadKo)
	InsmodArgs        string `xml:"InsmodArgs"`        // Arguments for insmod command
	PostLoadCmd       string `xml:"PostLoadCmd"`       // Command to execute after driver loading (e.g., "ifconfig wlan0 up")
	Interface         string `xml:"Interface"`         // Network interface name (default: wlan0)
	DriverBackend     string `xml:"DriverBackend"`     // Driver backend (default: nl80211)
}

// GetPreloadKoList returns prerequisite ko files as a slice
func (k *KernelConfig) GetPreloadKoList() []string {
	if k.PreloadKo == "" {
		return nil
	}
	parts := strings.Split(k.PreloadKo, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// GetPreloadArgsList returns prerequisite ko arguments as a slice (comma-separated matching PreloadKo)
func (k *KernelConfig) GetPreloadArgsList() []string {
	if k.PreloadInsmodArgs == "" {
		return nil
	}
	parts := strings.Split(k.PreloadInsmodArgs, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		result = append(result, strings.TrimSpace(p))
	}
	return result
}

// APConfig contains structured AP mode configuration
type APConfig struct {
	Capabilities *APCapabilities `xml:"Capabilities"`
	APProfile    *APProfile      `xml:"APProfile"`
	Network      *APNetwork      `xml:"Network"`
	FixedParams  []Param         `xml:"FixedParams>Param"`
	Params       []Param         `xml:"Param"` // backward-compatible flat params
}

// APCapabilities defines what the hardware module supports
type APCapabilities struct {
	Bands        string `xml:"Bands"`        // e.g. "2.4G,5G"
	Modes        string `xml:"Modes"`        // e.g. "b,g,n,ac,ax"
	MaxBandwidth int    `xml:"MaxBandwidth"` // e.g. 40, 80, 160
	SupportDFS   bool   `xml:"SupportDFS"`
	SupportWPA3  bool   `xml:"SupportWPA3"`
}

// APProfile holds default AP parameters
type APProfile struct {
	SSID     string `xml:"SSID"`
	Password string `xml:"Password"`
	Security string `xml:"Security"` // Open, WPA2, WPA3
	Band     string `xml:"Band"`     // 2.4G, 5G
	Channel  int    `xml:"Channel"`
}

// APNetwork holds DHCP / network configuration
type APNetwork struct {
	Gateway   string `xml:"Gateway"`
	Netmask   string `xml:"Netmask"`
	DHCPStart string `xml:"DHCPStart"`
	DHCPEnd   string `xml:"DHCPEnd"`
}

// Param represents a key-value configuration parameter
type Param struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}

// GetInterface returns the interface name by dynamically detecting it from the driver,
// defaulting to the configured interface or "wlan0" if empty.
func (k *KernelConfig) GetInterface() string {
	// 1. Dynamically find the interface based on the kernel module name
	if k.KoName != "" {
		driverName := strings.ReplaceAll(strings.TrimSuffix(k.KoName, ".ko"), "-", "_")
		iface := findInterfaceByDriver(driverName)
		if iface != "" {
			return iface
		}
	}

	// 2. Fallback to configured interface
	if k.Interface != "" {
		return k.Interface
	}

	// 3. Ultimate default
	return "wlan0"
}

func findInterfaceByDriver(driverName string) string {
	files, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return ""
	}
	var candidate string
	for _, f := range files {
		iface := f.Name()
		if iface == "lo" || iface == "sit0" || strings.HasPrefix(iface, "eth") ||
			strings.HasPrefix(iface, "p2p") || strings.HasPrefix(iface, "wifi-aware") ||
			strings.HasPrefix(iface, "mon") || strings.HasPrefix(iface, "apcli") {
			continue
		}

		matched := false
		// Check /sys/class/net/<iface>/device/driver symlink
		driverPath := filepath.Join("/sys/class/net", iface, "device", "driver")
		target, err := os.Readlink(driverPath)
		if err == nil {
			targetBase := filepath.Base(target)
			if targetBase == driverName || strings.ReplaceAll(targetBase, "-", "_") == driverName {
				matched = true
			}
		}

		// Also check /sys/class/net/<iface>/device/uevent for DRIVER=...
		if !matched {
			ueventPath := filepath.Join("/sys/class/net", iface, "device", "uevent")
			if data, err := os.ReadFile(ueventPath); err == nil {
				lines := strings.Split(string(data), "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "DRIVER=") {
						val := strings.TrimPrefix(line, "DRIVER=")
						if val == driverName || strings.ReplaceAll(val, "-", "_") == driverName {
							matched = true
							break
						}
					}
				}
			}
		}

		if matched {
			// Prefer wlanX over other matching interfaces
			if strings.HasPrefix(iface, "wlan") {
				return iface
			}
			if candidate == "" {
				candidate = iface
			}
		}
	}
	return candidate
}

// GetDriverBackend returns the driver backend, defaulting to "nl80211" if empty
func (k *KernelConfig) GetDriverBackend() string {
	if k.DriverBackend == "" {
		return "nl80211"
	}
	return k.DriverBackend
}

// ToMap converts a slice of Param to a map for easier lookup
func ParamsToMap(params []Param) map[string]string {
	m := make(map[string]string)
	for _, p := range params {
		m[p.Key] = p.Value
	}
	return m
}

// GetBands returns supported bands as a slice
func (c *APCapabilities) GetBands() []string {
	if c == nil || c.Bands == "" {
		return []string{"2.4G"}
	}
	return strings.Split(c.Bands, ",")
}

// GetModes returns supported WiFi modes as a slice
func (c *APCapabilities) GetModes() []string {
	if c == nil || c.Modes == "" {
		return []string{"b", "g", "n"}
	}
	return strings.Split(c.Modes, ",")
}

// HasBand checks if a band is supported
func (c *APCapabilities) HasBand(band string) bool {
	for _, b := range c.GetBands() {
		if strings.TrimSpace(b) == band {
			return true
		}
	}
	return false
}

// HasMode checks if a WiFi mode is supported
func (c *APCapabilities) HasMode(mode string) bool {
	for _, m := range c.GetModes() {
		if strings.TrimSpace(m) == mode {
			return true
		}
	}
	return false
}

// ExplicitlyHasMode checks if a WiFi mode is explicitly listed in the Modes field.
// Unlike HasMode, this does NOT fall back to defaults — returns false if Modes is empty.
func (c *APCapabilities) ExplicitlyHasMode(mode string) bool {
	if c == nil || c.Modes == "" {
		return false
	}
	for _, m := range strings.Split(c.Modes, ",") {
		if strings.TrimSpace(m) == mode {
			return true
		}
	}
	return false
}
