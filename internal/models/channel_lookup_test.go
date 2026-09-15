package models

import (
	"testing"
)

func TestChannelToFreq(t *testing.T) {
	tests := []struct {
		band     string
		ch       int
		expected int
	}{
		{"2.4G", 1, 2412},
		{"2.4G", 6, 2437},
		{"2.4G", 11, 2462},
		{"2.4G", 14, 2484},
		{"5G", 36, 5180},
		{"5G", 52, 5260},
		{"5G", 100, 5500},
		{"5G", 149, 5745},
		{"5G", 165, 5825},
		{"6G", 1, 5955},
		{"6G", 2, 5935},
		{"6G", 5, 5975},
		{"6G", 33, 6115},
		{"6G", 233, 7115},
	}

	for _, tt := range tests {
		got := ChannelToFreq(tt.band, tt.ch)
		if got != tt.expected {
			t.Errorf("ChannelToFreq(%s, %d) = %d, expected %d", tt.band, tt.ch, got, tt.expected)
		}
	}
}

func TestIsDFSChannel(t *testing.T) {
	if IsDFSChannel(36) {
		t.Errorf("Channel 36 should not be DFS")
	}
	if !IsDFSChannel(52) {
		t.Errorf("Channel 52 should be DFS")
	}
	if !IsDFSChannel(144) {
		t.Errorf("Channel 144 should be DFS")
	}
	if IsDFSChannel(149) {
		t.Errorf("Channel 149 should not be DFS")
	}
}

func TestLookupChannels(t *testing.T) {
	db, err := LoadChannelLookup()
	if err != nil {
		t.Fatalf("LoadChannelLookup failed: %v", err)
	}

	// Test US
	chansUS24 := db.LookupChannels("US", "所有类型AP", "2.4G", 20)
	if len(chansUS24) == 0 {
		t.Errorf("US 2.4G 20M channels should not be empty")
	}

	chansUS5 := db.LookupChannels("US", "所有类型AP", "5G", 20)
	if len(chansUS5) == 0 {
		t.Errorf("US 5G 20M channels should not be empty")
	}

	chansUS6 := db.LookupChannels("US", "所有类型AP", "6G", 20)
	if len(chansUS6) == 0 {
		t.Errorf("US 6G 20M channels should not be empty")
	}

	// Test TW
	chansTW24 := db.LookupChannels("TW", "所有类型AP", "2.4G", 20)
	if len(chansTW24) == 0 {
		t.Errorf("TW 2.4G 20M channels should not be empty")
	}
}
