package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestKeysConsistency(t *testing.T) {
	// Check all zh keys exist in en
	for k, zhVal := range zhMessages {
		enVal, ok := enMessages[k]
		if !ok {
			t.Errorf("Key '%s' found in zhMessages but missing in enMessages", k)
			continue
		}

		// Count % formatting placeholders
		zhCount := strings.Count(zhVal, "%") - 2*strings.Count(zhVal, "%%")
		enCount := strings.Count(enVal, "%") - 2*strings.Count(enVal, "%%")
		if zhCount != enCount {
			t.Errorf("Key '%s' placeholder count mismatch: zh has %d, en has %d\n  zh: %s\n  en: %s", k, zhCount, enCount, zhVal, enVal)
		}
	}

	// Check all en keys exist in zh
	for k := range enMessages {
		if _, ok := zhMessages[k]; !ok {
			t.Errorf("Key '%s' found in enMessages but missing in zhMessages", k)
		}
	}
}

func TestAllUsedKeysExist(t *testing.T) {
	// Scan all .go files in repository
	keyRegex := regexp.MustCompile(`i18n\.(?:T|Tf)\(\s*"([^"]+)"`)
	
	err := filepath.Walk("../..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		matches := keyRegex.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			if len(m) > 1 {
				k := m[1]
				if _, ok := zhMessages[k]; !ok {
					t.Errorf("File %s calls unknown i18n key '%s'", path, k)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Walk error: %v", err)
	}
}
