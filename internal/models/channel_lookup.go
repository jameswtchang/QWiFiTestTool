package models

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed country_channel_lookup.json
var countryChannelJSON []byte

// ChannelLookupDB represents the country channel lookup database
type ChannelLookupDB struct {
	Metadata  map[string]string `json:"metadata"`
	Countries []CountryEntry    `json:"countries"`
}

// CountryEntry represents a single country's channel rules
type CountryEntry struct {
	CountryName string                            `json:"国家/地区"`
	CountryEN   string                            `json:"Country/Region"`
	CountryCode string                            `json:"Country/Region Code"`
	APTypes     map[string]map[string]interface{} `json:"AP类型"`
}

// LoadChannelLookup loads and parses the embedded country channel JSON
func LoadChannelLookup() (*ChannelLookupDB, error) {
	var db ChannelLookupDB
	if err := json.Unmarshal(countryChannelJSON, &db); err != nil {
		return nil, fmt.Errorf("failed to parse channel lookup JSON: %w", err)
	}
	return &db, nil
}

// ListCountryCodes returns all available country codes sorted
func (db *ChannelLookupDB) ListCountryCodes() []string {
	codes := make([]string, 0, len(db.Countries))
	for _, c := range db.Countries {
		codes = append(codes, c.CountryCode)
	}
	sort.Strings(codes)
	return codes
}

// FindCountry returns the CountryEntry for a given country code
func (db *ChannelLookupDB) FindCountry(code string) *CountryEntry {
	for i := range db.Countries {
		if strings.EqualFold(db.Countries[i].CountryCode, code) {
			return &db.Countries[i]
		}
	}
	return nil
}

// GetAPTypes returns available AP types for this country (e.g. "Indoor AP", "Outdoor AP", "All Types AP")
func (c *CountryEntry) GetAPTypes() []string {
	types := make([]string, 0, len(c.APTypes))
	for t := range c.APTypes {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// LookupChannels queries available channels based on country, AP type, band, and bandwidth
func (db *ChannelLookupDB) LookupChannels(countryCode, apType, band string, bandwidth int) []int {
	country := db.FindCountry(countryCode)
	if country == nil {
		return nil
	}

	// Try exact AP type first, then fall back to "All Types AP"
	apData, ok := country.APTypes[apType]
	if !ok {
		apData, ok = country.APTypes["所有类型AP"]
		if !ok {
			// Try any available type
			for _, v := range country.APTypes {
				apData = v
				break
			}
		}
	}

	if apData == nil {
		return nil
	}

	// Build the lookup keys (may be multiple for 40MHz: 40M+ and 40M-)
	keys := buildChannelKeys(band, bandwidth)

	// Merge channels from all matching keys
	seen := make(map[int]bool)
	var channels []int
	for _, key := range keys {
		if val, ok := apData[key]; ok {
			for _, ch := range parseChannelValue(val) {
				if !seen[ch] {
					seen[ch] = true
					channels = append(channels, ch)
				}
			}
		}
	}

	sort.Ints(channels)
	return channels
}

// buildChannelKeys returns JSON keys for channel lookup
// For 40MHz, returns both 40M+ and 40M- keys to merge all valid channels
func buildChannelKeys(band string, bandwidth int) []string {
	switch {
	case band == "2.4G" && bandwidth == 20:
		return []string{"2.4G 20M 信道"}
	case band == "2.4G" && bandwidth == 40:
		return []string{"2.4G 40M+ 信道", "2.4G 40M- 信道"}
	case band == "5G" && bandwidth == 20:
		return []string{"5G 20M 信道"}
	case band == "5G" && bandwidth == 40:
		return []string{"5G 40M+ 信道", "5G 40M- 信道"}
	case band == "5G" && bandwidth == 80:
		return []string{"5G 80M 信道1"}
	case band == "5G" && bandwidth == 160:
		return []string{"5G 160M 信道2"}
	case band == "6G" && bandwidth == 20:
		return []string{"6G 20M 信道"}
	case band == "6G" && bandwidth == 40:
		return []string{"6G 40M 信道", "6G 40M+ 信道", "6G 40M- 信道"}
	case band == "6G" && bandwidth == 80:
		return []string{"6G 80M 信道", "6G 80M 信道1"}
	case band == "6G" && bandwidth == 160:
		return []string{"6G 160M 信道", "6G 160M 信道2"}
	case band == "6G" && bandwidth == 320:
		return []string{"6G 320M 信道"}
	default:
		return []string{fmt.Sprintf("%s %dM 信道", band, bandwidth)}
	}
}

// parseChannelValue parses a channel value from JSON which can be:
// - a string like "1-13" or "36, 40, 44, 48"
// - a number like 36
func parseChannelValue(val interface{}) []int {
	switch v := val.(type) {
	case string:
		return ParseChannelString(v)
	case float64:
		return []int{int(v)}
	case int:
		return []int{v}
	default:
		return nil
	}
}

// ParseChannelString parses channel strings like "1-13" or "36, 52, 100, 116"
// Supports mixed formats: "1-5, 10, 12-14"
func ParseChannelString(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}

	var channels []int
	parts := strings.Split(s, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check if it's a range (e.g., "1-13")
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				if err1 == nil && err2 == nil {
					for i := start; i <= end; i++ {
						channels = append(channels, i)
					}
				}
			}
		} else {
			// Single channel number
			if ch, err := strconv.Atoi(part); err == nil {
				channels = append(channels, ch)
			}
		}
	}

	return channels
}

// ChannelToFreq converts a WiFi channel number to its frequency in MHz
func ChannelToFreq(band string, ch int) int {
	if band == "2.4G" {
		if ch >= 1 && ch <= 13 {
			return 2407 + (ch * 5)
		}
		if ch == 14 {
			return 2484
		}
	} else if band == "5G" {
		return 5000 + (ch * 5)
	} else if band == "6G" {
		if ch == 2 {
			return 5935
		}
		return 5950 + (ch * 5)
	}
	return 0
}

// IsDFSChannel returns true if the 5GHz channel is a DFS channel (usually 52-144)
func IsDFSChannel(ch int) bool {
	return ch >= 52 && ch <= 144
}
