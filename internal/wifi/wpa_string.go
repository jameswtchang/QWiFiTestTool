package wifi

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// decodeWpaString reverses wpa_supplicant's printf_encode(), which wpa_cli applies to
// SSIDs in scan_results / status output: non-printable and non-ASCII bytes become \xNN
// (so UTF-8 "中文" is shown as \xe4\xb8\xad\xe6\x96\x87), plus \\ \" \n \r \t \e escapes.
func decodeWpaString(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}

	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			out = append(out, s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case '\\', '"':
			out = append(out, c)
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'e':
			out = append(out, 0x1b)
		case 'x':
			// Up to two hex digits, as in printf_decode()
			end := i + 1
			for end < len(s) && end < i+3 && isHexDigit(s[end]) {
				end++
			}
			if end == i+1 {
				out = append(out, 'x') // "\x" without digits: keep literal x
				continue
			}
			v, _ := strconv.ParseUint(s[i+1:end], 16, 8)
			out = append(out, byte(v))
			i = end - 1
		case '0', '1', '2', '3', '4', '5', '6', '7':
			// Up to three octal digits
			end := i
			for end < len(s) && end < i+3 && s[end] >= '0' && s[end] <= '7' {
				end++
			}
			v, _ := strconv.ParseUint(s[i:end], 8, 16)
			out = append(out, byte(v))
			i = end - 1
		default:
			// Unknown escape: printf_decode drops the backslash and keeps the char
			out = append(out, c)
		}
	}
	return string(out)
}

// formatSSIDValue returns the wpa_supplicant.conf value for ssid=.
// Plain printable ASCII stays quoted for readability; anything else (UTF-8, control
// characters, quotes) is written as unquoted hex, which wpa_supplicant parses byte-exact.
func formatSSIDValue(ssid string) string {
	for i := 0; i < len(ssid); i++ {
		if c := ssid[i]; c < 0x20 || c > 0x7e || c == '"' {
			return hex.EncodeToString([]byte(ssid))
		}
	}
	return `"` + ssid + `"`
}

// formatPSKValue validates a WPA-PSK credential and returns the value for psk=.
// wpa_supplicant accepts an 8-63 byte passphrase (quoted) or a 64-digit hex PSK (unquoted);
// anything else makes it reject the whole config file and exit at startup.
func formatPSKValue(password string) (string, error) {
	if len(password) == 64 && isHexString(password) {
		return password, nil
	}
	if len(password) < 8 || len(password) > 63 {
		return "", fmt.Errorf("WPA-PSK passphrase must be 8-63 characters or 64 hex digits (got %d bytes)", len(password))
	}
	if strings.ContainsAny(password, "\r\n") {
		return "", fmt.Errorf("WPA-PSK passphrase must not contain line breaks")
	}
	return `"` + password + `"`, nil
}

// isEnterpriseOnly reports whether scan flags advertise 802.1X (EAP) authentication without
// a PSK/SAE alternative (e.g. "[WPA2-EAP-CCMP][ESS]"). Such networks need identity/EAP
// settings this tool does not configure, so connecting with psk= can never succeed.
func isEnterpriseOnly(flags string) bool {
	u := strings.ToUpper(flags)
	return strings.Contains(u, "EAP") && !strings.Contains(u, "PSK") && !strings.Contains(u, "SAE")
}

// validateCredential checks the password against the key management derived from scan flags,
// so the caller can reject it before tearing down the running daemon.
func validateCredential(password, flags string) error {
	if isEnterpriseOnly(flags) {
		return fmt.Errorf("802.1X (EAP) enterprise authentication is not supported")
	}
	if password == "" {
		return nil // Open network
	}
	if deriveKeyMgmt(flags) == "SAE" {
		if strings.ContainsAny(password, "\r\n") {
			return fmt.Errorf("SAE password must not contain line breaks")
		}
		return nil
	}
	_, err := formatPSKValue(password)
	return err
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return s != ""
}
