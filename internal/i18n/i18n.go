package i18n

import "fmt"

// Lang represents a supported language
type Lang string

const (
	ZhTW Lang = "zh" // 繁體中文 (預設)
	EnUS Lang = "en"  // English
)

var current Lang = ZhTW

// SetLang sets the current language
func SetLang(lang Lang) {
	if _, ok := messages[lang]; ok {
		current = lang
	}
}

// GetLang returns the current language
func GetLang() Lang { return current }

// messages[lang][key] = translated string
var messages = map[Lang]map[string]string{}

// T returns the translated string for the given key.
// Fallback chain: current lang → zh-TW → key itself.
func T(key string) string {
	if m, ok := messages[current]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	// Fallback to zh-TW
	if current != ZhTW {
		if m, ok := messages[ZhTW]; ok {
			if v, ok := m[key]; ok {
				return v
			}
		}
	}
	return key
}

// Tf returns a formatted translated string (like fmt.Sprintf).
func Tf(key string, args ...interface{}) string {
	return fmt.Sprintf(T(key), args...)
}

func init() {
	messages[ZhTW] = zhMessages
	messages[EnUS] = enMessages
}
