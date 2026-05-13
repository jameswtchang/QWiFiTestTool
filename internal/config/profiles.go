package config

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed hostapd_profiles/*.conf
var profilesFS embed.FS

// HostapdProfile represents a parsed hostapd profile
type HostapdProfile struct {
	Name         string
	FileContent  string
	ParsedParams map[string]string
}

// GetAvailableProfiles returns a sorted list of available embedded profiles
func GetAvailableProfiles() ([]HostapdProfile, error) {
	entries, err := fs.ReadDir(profilesFS, "hostapd_profiles")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded profiles directory: %w", err)
	}

	var profiles []HostapdProfile
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".conf") {
			content, err := fs.ReadFile(profilesFS, "hostapd_profiles/"+entry.Name())
			if err != nil {
				continue
			}

			strContent := string(content)
			profile := HostapdProfile{
				Name:         strings.TrimSuffix(entry.Name(), ".conf"),
				FileContent:  strContent,
				ParsedParams: parseProfileParams(strContent),
			}
			profiles = append(profiles, profile)
		}
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
}

func parseProfileParams(content string) map[string]string {
	params := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			value := strings.TrimSpace(parts[1])
			// Strip inline comments (e.g. "1  # 代表 80 MHz")
			if idx := strings.Index(value, "#"); idx > 0 {
				value = strings.TrimSpace(value[:idx])
			}
			params[strings.TrimSpace(parts[0])] = value
		}
	}
	return params
}
