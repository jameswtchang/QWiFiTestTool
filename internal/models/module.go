package models

import "encoding/xml"

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
	APConfig      []Param      `xml:"APConfig>Param"`
}

// KernelConfig contains kernel-level module parameters
type KernelConfig struct {
	KoName        string `xml:"KoName"`        // Kernel module filename (e.g., 8821cu.ko)
	InsmodArgs    string `xml:"InsmodArgs"`    // Arguments for insmod command
	Interface     string `xml:"Interface"`     // Network interface name (default: wlan0)
	DriverBackend string `xml:"DriverBackend"` // Driver backend (default: nl80211)
}

// Param represents a key-value configuration parameter
type Param struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}

// GetInterface returns the interface name, defaulting to "wlan0" if empty
func (k *KernelConfig) GetInterface() string {
	if k.Interface == "" {
		return "wlan0"
	}
	return k.Interface
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
